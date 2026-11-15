import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Schemas } from "../api/client";
import { renderWithClient } from "../test-utils";
import { EmploymentCheck, monthYear } from "./EmploymentCheck";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const contact: Schemas["Contact"] = {
  id: "cont_1", version: 2, companyId: "comp_1", companyName: "Maju Sdn Bhd", firstName: "Nur", lastName: "Aina", fullName: "Nur Aina",
  title: "HR Manager", email: null, emailCheck: null, phone: null, mobile: null, linkedinUrl: null, status: "active", role: null, notes: null, mailboxCheck: null, linkedinCheck: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z", employment: null,
};
const moved: Schemas["ContactEmployment"] = {
  id: "cec_1", status: "read", verdict: "moved", fileName: "Profile.pdf", error: null, profileName: "Nur Aina", profileUrl: null,
  current: { title: "Talent Lead", company: "Contoh Bank Berhad", startDate: "2026-04", endDate: null, current: true },
  positions: [
    { title: "Talent Lead", company: "Contoh Bank Berhad", startDate: "2026-04", endDate: null, current: true },
    { title: "HR Manager", company: "Maju Sdn Bhd", startDate: "2021-03", endDate: "2026-03", current: false },
  ],
  nameMatches: true, urlMatches: null, checkedAt: "2026-11-06T00:00:00Z",
};

describe("employment check", () => {
  it("uploads the dropped LinkedIn PDF", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ ...moved, status: "reading" }), { status: 202 }));
    renderWithClient(<EmploymentCheck contact={contact} canCheck />);
    const input = screen.getByLabelText(/Drop their LinkedIn profile PDF/);
    fireEvent.change(input, { target: { files: [new File(["%PDF"], "Profile.pdf", { type: "application/pdf" })] } });
    await waitFor(() => { expect(fetch).toHaveBeenCalled(); });
    expect(fetch.mock.calls[0]?.[0]).toBe("/v1/contacts/cont_1/linkedin-profile");
  });

  it("says when the contact has moved", () => {
    renderWithClient(<EmploymentCheck contact={{ ...contact, employment: moved }} canCheck={false} />);
    expect(screen.getByText("No longer at Maju Sdn Bhd")).toBeTruthy();
    expect(screen.getByText(/now Talent Lead at Contoh Bank Berhad, since Apr 2026/)).toBeTruthy();
    expect(screen.getByText("Mar 2021 – Mar 2026")).toBeTruthy();
  });

  it("warns when the profile may be someone else's", () => {
    renderWithClient(<EmploymentCheck contact={{ ...contact, employment: { ...moved, nameMatches: false, profileName: "Lim Wei" } }} canCheck={false} />);
    expect(screen.getByText(/the name on it is Lim Wei/)).toBeTruthy();
  });

  it("formats months", () => {
    expect(monthYear("2026-04")).toBe("Apr 2026");
    expect(monthYear("2019")).toBe("2019");
    expect(monthYear(null)).toBe("");
  });
});
