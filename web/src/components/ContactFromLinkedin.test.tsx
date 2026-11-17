import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Contact } from "../api/client";
import * as companion from "../companion";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { ContactForm } from "./ContactForm";
import { afterContactAdded } from "./contactLinkedin";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const page = "https://www.linkedin.com/company/maju-bank/";
function serve() {
  vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: true, version: "1.3.0" });
  return vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: { q?: string } } }) => {
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    if (path === "/v1/companies") return Promise.resolve(ok({ items: [
      { id: "comp_2", name: "Maju Group", city: null, linkedinUrl: null },
      { id: "comp_1", name: "Maju Bank Berhad", city: "Kuala Lumpur", linkedinUrl: "linkedin.com/company/maju-bank" },
    ], nextCursor: null }));
    if (path === "/v1/contacts") return Promise.resolve(ok({ items: init?.params?.query?.q === "aisha@majubank.my"
      ? [{ id: "ct_7", fullName: "Aisha Rahman", companyName: "Maju Bank Berhad", email: "Aisha@MajuBank.my" }] : [], nextCursor: null }));
    return Promise.resolve(ok({}));
  }) as never);
}

describe("a new contact from LinkedIn", () => {
  it("fills in their name and title from the profile, and offers their company, the same LinkedIn page first", async () => {
    serve();
    vi.spyOn(companion, "companionCall").mockResolvedValue({ url: "https://www.linkedin.com/in/farah-aziz", name: "Farah binti Aziz",
      roles: [{ company_url: page, company_name: "Maju Bank", title: "Head of Talent Acquisition", since: "2023-03" }] });
    renderWithClient(<ContactForm contact={null} onDone={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("LinkedIn profile"), { target: { value: "linkedin.com/in/farah-aziz/" } });
    fireEvent.click(screen.getByRole("button", { name: "Fill in from LinkedIn" }));
    expect((await screen.findByRole("status")).textContent).toContain("Head of Talent Acquisition at Maju Bank since Mar 2023");
    expect(screen.getByLabelText<HTMLInputElement>("First name").value).toBe("Farah binti");
    expect(screen.getByLabelText<HTMLInputElement>("Last name").value).toBe("Aziz");
    expect(screen.getByLabelText<HTMLInputElement>("Job title").value).toBe("Head of Talent Acquisition");
    const offered = await screen.findAllByRole("button", { name: /Maju/ });
    expect(offered.map((b) => b.textContent)).toEqual(["Maju Bank Berhad (same LinkedIn page)", "Maju Group"]);
    fireEvent.click(offered[0] as HTMLElement);
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Maju Group" })).toBeNull(); }); // chosen
  });

  it("says at once when the email belongs to a contact already", async () => {
    serve();
    renderWithClient(<ContactForm contact={null} onDone={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "Aisha@majubank.my" } });
    const alert = await screen.findByRole("alert", {}, { timeout: 2000 });
    expect(alert.textContent).toBe("Already a contact: Aisha Rahman at Maju Bank Berhad.");
    expect(screen.getByRole("link", { name: "Aisha Rahman" }).getAttribute("href")).toBe("#/contacts/ct_7");
  });

  it("once saved: guesses the email, then checks their time at the company on LinkedIn", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ suggestion: { email: "farah.aziz@majubank.my", pattern: "first.last", matches: 3, of: 3 } }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({}));
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/contacts/linkedin-checks"
      ? { companyUrl: page, checks: [{ id: "lic_1", contactId: "ct_9", contactName: "Farah", profileUrl: "https://www.linkedin.com/in/farah-aziz" }], skipped: [] }
      : {}))) as never);
    vi.spyOn(companion, "companionCall").mockImplementation(((action: string) => Promise.resolve(action === "companyId"
      ? { company_id: "123" }
      : { url: "https://www.linkedin.com/in/farah-aziz", company_url: page, company_id: "123", joined: "2023-03", left: "", current: true })));
    await afterContactAdded({ id: "ct_9", version: 1, email: null, linkedinUrl: "https://www.linkedin.com/in/farah-aziz" } as Contact);
    expect(get).toHaveBeenCalledWith("/v1/contacts/{contactId}/email-suggestion", expect.anything());
    expect(sentBody(patch)).toEqual({ version: 1, email: "farah.aziz@majubank.my" });
    expect(sentBody(post, 1)).toEqual({ profileUrl: "https://www.linkedin.com/in/farah-aziz", companyUrl: page, linkedinCompanyId: "123", joined: "2023-03", left: "", current: true });
  });
});
