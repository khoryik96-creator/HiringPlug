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
};

describe("candidate list", () => {
  it("filters by skill and tag from the values already in use, and shows tags by the name", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => {
      if (path === "/v1/candidates/used-values") {
        return Promise.resolve(ok({ tags: ["relocate"], skills: ["SAP"], industries: [], specializations: [] }));
      }
      return Promise.resolve(ok({ items: [summary], nextCursor: null }));
    }));
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
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/candidates"
      ? { items: rows, nextCursor: null } : { tags: [], skills: [], industries: [], specializations: [] }))) as never);
    renderWithClient(<CandidateList canCreate={false} />);
    const siti = (await screen.findByText("Siti Aisyah")).closest("tr");
    expect(siti?.textContent).toContain("Not looking");
    expect(siti?.textContent).toContain("· 5 days");
    expect(siti?.textContent).toContain("1 month");
    expect(siti?.textContent).toContain("8,500");
    expect(siti?.textContent).toContain("12,000");
    expect(screen.getByText("Ali").closest("tr")?.textContent).toContain("Restricted");
  });
});
