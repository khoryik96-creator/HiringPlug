import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const summary = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Siti Aisyah", email: null, phone: null, currentTitle: "Accountant",
  currentCompany: null, location: null, workAuthorizationStatus: null, tags: ["relocate"], updatedAt: "2026-11-01T00:00:00Z",
  availability: null, availabilitySince: null, noticePeriod: null, currentSalary: null, expectedSalary: null, salaryRestricted: false,
  workAuthorizationBasis: null, nationality: null, residenceCountry: null, source: null, industry: null,
};

/** The list's requests: the candidates, the values in use, the person's columns (default unless given) and the settings. */
function answer(items: unknown[], columns: string[] | null = null) {
  return ((path: string) => {
    if (path === "/v1/candidates") return Promise.resolve(ok({ items, nextCursor: null }));
    if (path === "/v1/me/preferences") return Promise.resolve(ok({ candidateColumns: columns }));
    if (path === "/v1/settings/fields") return Promise.resolve(ok({ lists: [] }));
    return Promise.resolve(ok({ tags: ["relocate"], skills: ["SAP"], industries: [], specializations: [] }));
  }) as never;
}

describe("candidate list", () => {
  it("filters by skill and tag from the values already in use, and shows tags by the name", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList canCreate={false} />);
    expect(await screen.findByText("Siti Aisyah")).toBeTruthy();
    expect(screen.getAllByText("relocate").some((el) => el.classList.contains("chip"))).toBe(true);

    fireEvent.change(await screen.findByLabelText("Skill"), { target: { value: "SAP" } });
    await screen.findByRole("option", { name: "relocate" });
    fireEvent.change(screen.getByLabelText("Tag"), { target: { value: "relocate" } });
    await waitFor(() => {
      expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { skill: "SAP", tag: "relocate" } } });
    });
  });

  it("shows each candidate's status and since when, notice period and salaries, or that salary is restricted", async () => {
    const since = new Date(Date.now() - 5 * 86_400_000).toISOString();
    const rows = [
      { ...summary, availability: "not_looking", availabilitySince: since, noticePeriod: { value: 1, unit: "month" },
        currentSalary: { amount: "8500.00", currency: "MYR", period: "month" },
        expectedSalary: { min: "10000.00", max: "12000.00", currency: "MYR", period: "month" } },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Ali", salaryRestricted: true },
    ];
    vi.spyOn(api, "GET").mockImplementation(answer(rows));
    renderWithClient(<CandidateList canCreate={false} />);
    const siti = (await screen.findByText("Siti Aisyah")).closest("tr");
    expect(siti?.textContent).toContain("Not looking");
    expect(siti?.textContent).toContain("· 5 days");
    expect(siti?.textContent).toContain("1 month");
    expect(siti?.textContent).toContain("8,500");
    expect(siti?.textContent).toContain("12,000");
    expect(screen.getByText("Ali").closest("tr")?.textContent).toContain("Restricted");
  });

  it("marks each name with a bar coloured by work authorization", async () => {
    const rows = [
      { ...summary, workAuthorizationStatus: "local" },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Ali", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "rp_t" },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w9", fullName: "Mei", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "permanent_resident" },
    ];
    vi.spyOn(api, "GET").mockImplementation(answer(rows));
    renderWithClient(<CandidateList canCreate={false} />);
    await screen.findByText("Mei");
    expect(screen.getByRole("img", { name: "Work authorization: Local" }).closest("tr")?.textContent).toContain("Siti Aisyah");
    expect(screen.getByRole("img", { name: "Work authorization: RP-T" }).closest("tr")?.textContent).toContain("Ali");
    expect(screen.getByRole("img", { name: "Work authorization: PR" }).closest("tr")?.textContent).toContain("Mei");
  });

  it("shows the columns the person chose, and saves a new choice for them", async () => {
    const row = { ...summary, nationality: "SG", residenceCountry: "MY", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "spouse_visa" };
    vi.spyOn(api, "GET").mockImplementation(answer([row], ["nationality", "residence", "workAuthorization"]));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ candidateColumns: ["nationality", "residence", "workAuthorization", "industry"] }));
    renderWithClient(<CandidateList canCreate={false} />);
    await screen.findByRole("columnheader", { name: "Nationality" });
    expect(screen.getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["Name", "Nationality", "Residence", "Work authorization"]);
    const siti = screen.getByText("Siti Aisyah").closest("tr");
    expect(siti?.textContent).toContain("Singapore");
    expect(siti?.textContent).toContain("Malaysia");
    expect(siti?.textContent).toContain("Spouse visa");

    fireEvent.click(screen.getByRole("button", { name: "Columns" }));
    fireEvent.click(screen.getByLabelText("Industry"));
    await waitFor(() => {
      expect(put).toHaveBeenCalledWith("/v1/me/preferences", { body: { candidateColumns: ["nationality", "residence", "workAuthorization", "industry"] } });
    });
    expect(await screen.findByRole("columnheader", { name: "Industry" })).toBeTruthy();
  });
});
