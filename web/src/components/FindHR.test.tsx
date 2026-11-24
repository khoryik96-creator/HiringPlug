import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company, type Schemas } from "../api/client";
import * as companion from "../companion";
import { ok, renderWithClient } from "../test-utils";
import { FindHR } from "./FindHR";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const company = { id: "comp_1", name: "Maju Bank Berhad", linkedinUrl: "https://www.linkedin.com/company/maju-bank/" } as Company;
const person = (over: Partial<Schemas["HrScanPerson"]>): Schemas["HrScanPerson"] => ({
  url: "https://www.linkedin.com/in/farah", name: "Farah Aziz", title: "Head of Talent Acquisition", location: "Kuala Lumpur", tier: "A",
  hidden: null, joined: "2023-03", left: null, current: true, contactId: null, contactName: null, duplicateOf: null, duplicateName: null, match: null, ...over,
});
const match = (over: Partial<Schemas["HrScanMatch"]>): Schemas["HrScanMatch"] => ({
  kind: "profile", contactId: "ct_1", name: "Kumar Selvam", title: "HR Director", companyId: "comp_2", companyName: "Contoh Holdings",
  sameCompany: false, email: "kumar@contoh.example", emailCheckStatus: "valid", emailPersonal: false, left: false, more: 0, ...over,
});
const scan: Schemas["HrScan"] = {
  companyId: "comp_1", linkedinCompanyId: "123", linkedinCompanyName: "Maju Bank", country: "MY", countryName: "Malaysia", geoId: "106808692",
  keywords: ["HR"], queries: ['"HR"'], pages: 3, status: "done", next: null, pagesRead: 3, error: null, startedAt: "2026-11-17T00:00:00Z", updatedAt: "2026-11-17T00:00:00Z",
  people: [
    person({}),
    person({ url: "https://www.linkedin.com/in/daniel", name: "Daniel Lee", title: "HR Business Partner", tier: "B", joined: null, current: null }),
    person({ url: "https://www.linkedin.com/in/kumar", name: "Kumar Selvam", tier: "B", contactId: "ct_1", contactName: "Kumar Selvam", match: match({}) }),
    person({ url: "https://www.linkedin.com/in/aisha", name: "Aisha Rahman", tier: "A", duplicateOf: "ct_2", duplicateName: "Nur Aisha binti Rahman",
      match: match({ kind: "similar_name", contactId: "ct_2", name: "Nur Aisha binti Rahman", title: null, companyId: "comp_1", companyName: "Maju Bank Berhad",
        sameCompany: true, email: "aisha@majubank.my", emailCheckStatus: "risky", left: true, more: 1 }) }),
    person({ url: "https://www.linkedin.com/in/nurul", name: "Nurul Huda", title: "HR Intern", tier: "C", hidden: "an intern or trainee" }),
  ],
};

function serve() {
  vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: true, version: "1.2.0" });
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path.endsWith("/email-suggestion")
    ? { suggestion: { email: "daniel.lee@majubank.my", pattern: "first.last", matches: 3, of: 3 } }
    : { scan }))) as never);
}

describe("Find HR", () => {
  it("lists who it found, best first, with who is a contact already or may be, and joining dates", async () => {
    serve();
    renderWithClient(<FindHR company={company} canCreate canUseApollo={false} />);
    expect((await screen.findByRole("status")).textContent).toContain("Done · 3 pages read · 5 found");
    const farah = screen.getByRole("link", { name: "Farah Aziz" });
    expect(farah.getAttribute("href")).toBe("https://www.linkedin.com/in/farah");
    expect(farah.getAttribute("target")).toBe("_blank");
    expect(farah.closest("tr")?.textContent).toContain("Mar 2023");
    // Who they are in HiringPlug, wherever in the agency, with the email to use.
    const kumar = screen.getByRole("link", { name: "In HiringPlug" });
    expect(kumar.getAttribute("href")).toBe("#/contacts/ct_1");
    const kumarRow = kumar.closest("tr") as HTMLElement;
    expect(kumarRow.textContent).toContain("HR Director · at Contoh Holdings");
    expect(screen.getByRole("button", { name: "kumar@contoh.example: copy email" })).toBeTruthy();
    const aisha = screen.getByRole("link", { name: "Similar name" }).closest("td") as HTMLElement;
    expect(screen.getByRole("link", { name: "Nur Aisha binti Rahman" }).getAttribute("href")).toBe("#/contacts/ct_2");
    expect(aisha.textContent).toContain("left");
    expect(aisha.textContent).toContain("aisha@majubank.my may not arrive");
    expect(aisha.textContent).toContain("1 more contact may be them");
    expect(aisha.textContent).not.toContain("at Maju Bank Berhad"); // the company searched goes without saying
    expect(screen.getByText("1 in HiringPlug · 1 may be · 2 new · 2 with an email")).toBeTruthy();
    // Copying an email does not choose the row.
    fireEvent.click(screen.getByRole("button", { name: "aisha@majubank.my: copy email" }));
    expect(aisha.closest("tr")?.getAttribute("aria-selected")).toBe("false");
    expect(screen.queryByText("Nurul Huda")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /1 hidden/ }));
    expect(screen.getByText("Nurul Huda")).toBeTruthy();
  });

  it("offers no Apollo lookup for a work email HiringPlug has already", async () => {
    const known = person({ url: "https://www.linkedin.com/in/ana", name: "Ana Lim", contactId: "ct_3",
      match: match({ contactId: "ct_3", name: "Ana Lim", companyId: "comp_1", companyName: "Maju Bank Berhad", sameCompany: true, email: "ana@majubank.my" }) });
    const personal = person({ url: "https://www.linkedin.com/in/raj", name: "Raj Kumar", contactId: "ct_4",
      match: match({ contactId: "ct_4", name: "Raj Kumar", companyId: "comp_1", sameCompany: true, email: "raj@gmail.com", emailPersonal: true }) });
    vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: true, version: "1.2.0" });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/contacts/apollo"
      ? { state: "connected", accountName: "Recruiter", authorizationUrl: null }
      : { scan: { ...scan, people: [known, personal, person({})] } }))) as never);
    renderWithClient(<FindHR company={company} canCreate canUseApollo />);
    expect(await screen.findByRole("button", { name: "Work email for Farah Aziz" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Work email for Ana Lim" })).toBeNull();
    expect(screen.getByRole("button", { name: "Work email for Raj Kumar" }), "a personal address is not their work email").toBeTruthy();
    expect(screen.getByRole("button", { name: "raj@gmail.com: copy email" }).closest("td")?.textContent).toContain("personal");
  });

  it("chooses by clicking a row (not its name), All and None, and adds the chosen as contacts with a guessed email", async () => {
    serve();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "ct_9", version: 1 }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({}));
    renderWithClient(<FindHR company={company} canCreate canUseApollo={false} />);
    const daniel = (await screen.findByText("Daniel Lee")).closest("tr") as HTMLElement;
    fireEvent.click(screen.getByRole("link", { name: "Farah Aziz" }));
    expect(screen.queryByRole("button", { name: /Add \d+ as contacts/ })).toBeNull(); // a name opens LinkedIn, it does not choose
    fireEvent.click(daniel);
    expect(daniel.getAttribute("aria-selected")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "All 3" })); // not the contact already there
    expect(screen.getByRole("button", { name: "Add 3 as contacts" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "None" }));
    expect(screen.queryByRole("button", { name: /as contacts/ })).toBeNull();

    fireEvent.click(daniel);
    fireEvent.click(screen.getByRole("button", { name: "Add 1 as contacts" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect((post.mock.calls[0]?.[1] as unknown as { body: unknown }).body).toEqual({ companyId: "comp_1", firstName: "Daniel", lastName: "Lee",
      title: "HR Business Partner", linkedinUrl: "https://www.linkedin.com/in/daniel", role: "hr" }); // found as HR, tagged HR
    expect((patch.mock.calls[0]?.[1] as unknown as { body: unknown }).body).toEqual({ version: 1, email: "daniel.lee@majubank.my" });
  });

  it("reads joining dates only for those not read yet, so a second click carries on where a failure stopped", async () => {
    serve();
    let signIn = true;
    const call = vi.spyOn(companion, "companionCall").mockImplementation(() =>
      signIn ? Promise.reject(new Error("LinkedIn asked to sign in")) : Promise.resolve({ joined: "2024-01", left: "", current: true }));
    vi.spyOn(api, "POST").mockResolvedValue(ok(scan)); // the scan as saved (Daniel still without dates)
    renderWithClient(<FindHR company={company} canCreate canUseApollo={false} />);
    await screen.findByText("Daniel Lee");
    fireEvent.click(screen.getByRole("button", { name: "All 3" })); // Farah and Aisha read already, Daniel not
    fireEvent.click(screen.getByRole("button", { name: "Joining dates (1)" }));
    expect(await screen.findByText("LinkedIn asked to sign in")).toBeTruthy();
    signIn = false;
    fireEvent.click(screen.getByRole("button", { name: "Joining dates (1)" }));
    await waitFor(() => { expect(call).toHaveBeenCalledTimes(2); });
    // Only Daniel, both times: those whose dates are known are never read again.
    expect(call.mock.calls.map((c) => (c[1] as { url: string }).url)).toEqual(["https://www.linkedin.com/in/daniel", "https://www.linkedin.com/in/daniel"]);
    // Read now (even if the profile gave no dates here): not asked for again.
    await waitFor(() => { expect(screen.getAllByRole("button").map((b) => b.textContent)).toContain("Joining dates (0)"); });
  });

  it("a scan goes on while another tab is shown, and is picked up again", async () => {
    serve();
    const { activeRuns, newRun } = await import("./findHrRun");
    const run = newRun(vi.fn());
    activeRuns.set("comp_1", run);
    renderWithClient(<FindHR company={company} canCreate canUseApollo={false} />);
    expect(await screen.findByRole("button", { name: "Stop" })).toBeTruthy();
    activeRuns.delete("comp_1");
  });

  it("asks for the company's LinkedIn page first", () => {
    serve();
    renderWithClient(<FindHR company={{ ...company, linkedinUrl: null }} canCreate canUseApollo={false} />);
    expect(screen.getByText(/Add the company's LinkedIn page/)).toBeTruthy();
  });

  it("lays out the same scan for a narrow panel when compact", async () => {
    serve();
    renderWithClient(<FindHR company={company} canCreate canUseApollo={false} compact />);
    expect((await screen.findByRole("status")).textContent).toContain("Done · 3 pages read · 5 found");
    expect(screen.getByRole("region", { name: "Find HR" }).className).toBe("panel find-hr compact");
    expect(screen.queryByRole("heading", { name: /Find HR/ }), "the panel's own title names it").toBeNull();
    expect(screen.getByRole("link", { name: "Maju Bank Berhad on LinkedIn ↗" }).getAttribute("href")).toBe("https://www.linkedin.com/company/maju-bank/");
    expect(screen.getByRole("button", { name: "All 3" })).toBeTruthy();
  });
});
