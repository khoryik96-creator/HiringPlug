import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate, testFieldSettings } from "../test-utils";
import { CandidateBanner, livesIn } from "./CandidateBanner";
import { raiseText, sinceText, waColumnLabel, waFor, waKind } from "./candidateLook";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const settings = () => vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
const salary = {
  version: 1, updatedAt: "2026-11-01T00:00:00Z",
  currentSalary: { amount: "9500.00", currency: "MYR", period: "month" as const },
  expectedSalary: { min: "12000.00", max: null, currency: "MYR", period: "month" as const },
};

describe("work authorization colours", () => {
  it("map the stored status and pass to the five choices, and back", () => {
    expect(waKind(null)).toBe("unknown");
    expect(waKind({ status: "local" })).toBe("local");
    expect(waKind({ status: "expat_eligible", basis: "permanent_resident" })).toBe("pr");
    expect(waKind({ status: "expat_eligible", basis: "spouse_visa" })).toBe("expat");
    expect(waKind({ status: "requires_visa" })).toBe("needsPass");
    expect(waColumnLabel({ status: "expat_eligible", basis: "spouse_visa" })).toBe("Spouse visa");
    expect(waFor("pr", { status: "local", note: "Checked IC" })).toEqual({ status: "expat_eligible", basis: "permanent_resident", note: "Checked IC" });
    expect(waFor("unknown", { status: "local" })).toBeNull();
    expect(raiseText(salary)).toBe("+26% on current");
    expect(raiseText({ ...salary, expectedSalary: { ...salary.expectedSalary, currency: "SGD" } })).toBeNull();
    expect(raiseText({ ...salary, expectedSalary: { ...salary.expectedSalary, min: "9480.00" } })).toBe("About the same as current"); // not "+0%"
    expect(livesIn("Malaysia", "Malaysia")).toBe("Malaysia"); // not "Malaysia, Malaysia"
    expect(livesIn("Petaling Jaya", "Malaysia")).toBe("Petaling Jaya, Malaysia");
    expect(livesIn(null, "Singapore")).toBe("Singapore");
  });
});

describe("candidate banner", () => {
  it("is coloured by status, with the work authorization column, the facts and the figures", () => {
    settings();
    renderWithClient(<CandidateBanner canEdit={false} candidate={testCandidate({
      availability: "passive", currentCompany: "Maju Foods", location: "Petaling Jaya", residenceCountry: "MY",
      workAuthorization: { status: "local" }, skills: ["SAP FICO", "IFRS"], compensation: salary,
    })} />);
    const banner = screen.getByRole("region", { name: "Candidate" });
    expect(banner.style.background).toBe("rgb(205, 239, 200)"); // light green: passive
    expect(within(banner).getByText("Passive")).toBeTruthy();
    expect(within(banner).getByText("Accountant · Maju Foods")).toBeTruthy();
    expect(within(banner).getByText("Petaling Jaya, Malaysia")).toBeTruthy();
    expect(banner.textContent).toContain("Work authorization: Local");
    expect(banner.textContent).toContain("RM 9,500 / month");
    expect(banner.textContent).toContain("RM 12,000 / month");
    expect(banner.textContent).toContain("+26% on current");
    expect(within(banner).queryByRole("group", { name: "Status" })).toBeNull(); // no buttons without candidates:update
  });

  it("says since when the status is set, and for how many days", () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    const since = new Date(Date.now() - 12 * 86_400_000).toISOString();
    renderWithClient(<CandidateBanner candidate={testCandidate({ availability: "not_looking", availabilitySince: since })} canEdit={false} />);
    expect(screen.getByText(/^since .* · 12 days$/)).toBeTruthy();
    expect(sinceText("2026-11-03T02:00:00Z", new Date("2026-11-03T09:00:00Z"))).toBe("since 3 Nov 2026 · today");
    expect(sinceText("2026-11-02T02:00:00Z", new Date("2026-11-03T09:00:00Z"))).toBe("since 2 Nov 2026 · 1 day");
    expect(sinceText(null)).toBeNull();
    // Calendar days, as the date says: set late yesterday is "1 day", not "today".
    expect(sinceText(new Date(2026, 10, 7, 23, 0).toISOString(), new Date(2026, 10, 8, 9, 0))).toBe("since 7 Nov 2026 · 1 day");
  });

  it("says when the salary is restricted for the role", () => {
    settings();
    renderWithClient(<CandidateBanner canEdit={false} candidate={testCandidate({ restrictedFields: ["compensation"] })} />);
    expect(screen.getAllByText("Restricted")).toHaveLength(2);
  });

  it("saves a status at once with the version shown; picking it again clears it", async () => {
    settings();
    const c = testCandidate({ availability: "looking", version: 3 });
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...c, availability: "placed", version: 4 }));
    renderWithClient(<CandidateBanner canEdit candidate={c} />);
    const status = screen.getByRole("group", { name: "Status" });
    expect(within(status).getByRole("button", { name: "Looking out" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(status).getByRole("button", { name: "Placed" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 3, availability: "placed" });
    fireEvent.click(within(status).getByRole("button", { name: "Looking out" }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
    expect(sentBody(patch, 1)).toEqual({ version: 3, availability: null });
  });

  it("asks which pass for Spousal / RP-T, and keeps the note", async () => {
    settings();
    const c = testCandidate({ workAuthorization: { status: "local", note: "IC seen" } });
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(c));
    renderWithClient(<CandidateBanner canEdit candidate={c} />);
    const wa = screen.getByRole("group", { name: "Work authorization" });
    fireEvent.click(within(wa).getByRole("button", { name: "Spousal / RP-T" }));
    expect(patch).not.toHaveBeenCalled();
    fireEvent.click(within(screen.getByRole("group", { name: "Pass held" })).getByRole("button", { name: "Spouse visa" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ workAuthorization: { status: "expat_eligible", basis: "spouse_visa", note: "IC seen" } });

    fireEvent.click(within(wa).getByRole("button", { name: "Needs pass" }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
    expect(sentBody(patch, 1)).toMatchObject({ workAuthorization: { status: "requires_visa", basis: null } });
  });
});
