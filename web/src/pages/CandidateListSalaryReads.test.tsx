// A page sorted or found by salary is counted in the log of who was shown a
// salary, salary columns or not: it is never read again just because the
// window got focus.
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { focusManager } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  localStorage.removeItem("hiringplug.candidates.sort");
  focusManager.setFocused(undefined);
  vi.restoreAllMocks();
});

const summary = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Siti Aisyah", email: null, phone: null, currentTitle: "Accountant",
  currentCompany: null, location: null, workAuthorizationStatus: null, tags: [], updatedAt: "2026-11-01T00:00:00Z",
  availability: null, availabilitySince: null, noticePeriod: null, currentSalary: null, expectedSalary: null, salaryRestricted: false,
  workAuthorizationBasis: null, nationality: null, residenceCountry: null, source: null, industry: null,
};

/** The list's requests; the person's columns have no salary column. */
const answer = ((path: string) => {
  if (path === "/v1/candidates") return Promise.resolve(ok({ items: [summary], nextCursor: null }));
  if (path === "/v1/me/preferences") return Promise.resolve(ok({ candidateColumns: ["currentRole", "status", "email"] }));
  if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
  return Promise.resolve(ok({ tags: [], skills: [], industries: [], specializations: [], certifications: [] }));
}) as never;

/** The window loses focus and gets it back (the queries that may are read again). */
async function refocus() {
  focusManager.setFocused(false);
  focusManager.setFocused(true);
  await new Promise((r) => setTimeout(r, 50));
}

describe("candidate pages counted as salary views", () => {
  it("are never read again on focus when sorted or found by salary, salary columns or not", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(answer);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [{ candidate: summary, matchedIn: [], cvSnippet: null }], nextCursor: null, terms: [] }));
    const lists = () => get.mock.calls.filter((c) => c[0] === "/v1/candidates").length;
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByRole("link", { name: "Siti Aisyah" });
    await refocus();
    await waitFor(() => { expect(lists(), "a list that tells nothing about salaries is read again on focus").toBe(2); });
    cleanup();

    get.mockClear();
    localStorage.setItem("hiringplug.candidates.sort", "expected_salary_desc");
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByRole("link", { name: "Siti Aisyah" });
    expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { sort: "expected_salary_desc" } } }); // no salary column: no salaries
    await refocus();
    expect(lists(), "sorted by salary").toBe(1);

    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    fireEvent.change(screen.getByLabelText(/Expects at most/), { target: { value: "12,000" } });
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
    expect(sentBody(post)).toMatchObject({ expectedSalaryMax: { amount: "12000", currency: "MYR" } });
    expect(sentBody(post)).not.toHaveProperty("includeSalary");
    await refocus();
    expect(post, "found by the expected salary").toHaveBeenCalledTimes(1);
  });
});
