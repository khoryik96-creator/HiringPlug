// A candidate or screening calls that came back with a salary are counted as
// shown each time they are read: not read again just because the window got
// focus. Without a salary they are, as every page is.
import { cleanup, screen } from "@testing-library/react";
import { focusManager } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ScreeningCalls } from "../components/ScreeningCalls";
import { CandidateDetail } from "../pages/CandidateDetail";
import { ok, RECRUITER, renderWithClient, testCandidate } from "../test-utils";
import { api, type Candidate } from "./client";

afterEach(() => {
  cleanup();
  focusManager.setFocused(undefined);
  vi.restoreAllMocks();
});

/** The window loses focus and gets it back (the queries that may are read again). */
async function refocus() {
  focusManager.setFocused(false);
  focusManager.setFocused(true);
  await new Promise((r) => setTimeout(r, 50));
}

const salary = { currentSalary: { amount: "8517.00", currency: "MYR", period: "month", description: null }, expectedSalary: null, salaryNotes: null, version: 1 };

describe("answers with a salary are not read again on focus", () => {
  for (const [what, compensation, reads] of [["with a salary", salary, 1], ["without one", null, 2]] as const) {
    it(`a candidate's page ${what}`, async () => {
      const c = { ...testCandidate(), compensation } as Candidate;
      const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => {
        if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(c));
        if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: [] }));
        return Promise.reject(new Error(`unexpected GET ${path}`));
      }) as never);
      renderWithClient(<CandidateDetail id={c.id} permissions={RECRUITER} />);
      await screen.findByRole("heading", { name: c.fullName });
      await refocus();
      expect(get.mock.calls.filter((call) => call[0] === "/v1/candidates/{candidateId}")).toHaveLength(reads);
    });
  }

  for (const [what, snapshot, reads] of [["with a salary", { currentSalary: salary.currentSalary, expectedSalary: null, salaryNotes: null }, 1], ["without one", undefined, 2]] as const) {
    it(`a candidate's screening calls ${what}`, async () => {
      const call = { id: "scl_1", calledOn: "2026-11-08", calledBy: "usr_1", calledByName: "Aina", noteId: null, salaryRecorded: true,
        createdAt: "2026-11-08T08:00:00Z", ...(snapshot ? { salarySnapshot: snapshot } : {}) };
      const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [call] }));
      renderWithClient(<ScreeningCalls candidateId="cand_1" permissions={["candidates:read", "candidates.compensation:read"]} />);
      expect(await screen.findByText(/8 Nov 2026/)).toBeTruthy();
      await refocus();
      expect(get.mock.calls.filter((c) => c[0] === "/v1/candidates/{candidateId}/screening-calls")).toHaveLength(reads);
    });
  }
});
