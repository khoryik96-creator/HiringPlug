import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Candidate, type Permission } from "../api/client";
import { candidateKey } from "../api/keys";
import { failure, ok, RECRUITER, renderWithClient, sentBody, testCandidate, testFieldSettings } from "../test-utils";
import { CandidateDetail } from "./CandidateDetail";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

/** Answers the page's reads: the candidate (or `candidateResult`) and an empty document list. */
function mockReads(candidateResult: () => unknown) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/candidates/{candidateId}") return Promise.resolve(candidateResult());
    if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: [] }));
    return Promise.reject(new Error(`unexpected GET ${path}`));
  }) as never);
}

async function show(candidate: Candidate, permissions: Permission[]) {
  mockReads(() => ok(candidate));
  const view = renderWithClient(<CandidateDetail id={candidate.id} permissions={permissions} />);
  await screen.findByRole("heading", { name: candidate.fullName });
  return view;
}

const buttons = () => screen.queryAllByRole("button").map((b) => b.textContent);

describe("candidate page", () => {
  it("gives viewers no edit buttons", async () => {
    await show(testCandidate(), ["candidates:read", "candidates.compensation:read", "documents:read"]);
    expect(buttons()).not.toContain("Edit profile");
    expect(buttons()).not.toContain("Add salary");
  });

  it("lets recruiters edit the profile and the salary", async () => {
    await show(testCandidate(), RECRUITER);
    expect(buttons()).toContain("Edit profile");
    expect(buttons()).toContain("Add salary");
  });

  it("lets sourcers edit the profile, but not see or edit salary", async () => {
    await show(testCandidate({ restrictedFields: ["compensation"] }), ["candidates:read", "candidates:update", "documents:read"]);
    expect(buttons()).toContain("Edit profile");
    expect(screen.getByText(/restricted for your role/)).toBeTruthy();
    expect(buttons()).not.toContain("Add salary");
  });

  it("keeps an open form and what was typed when a background refresh fails", async () => {
    const candidate = testCandidate();
    let next: unknown = ok(candidate);
    mockReads(() => next);
    const { client } = renderWithClient(<CandidateDetail id={candidate.id} permissions={RECRUITER} />);
    fireEvent.click(await screen.findByRole("button", { name: "Edit profile" }));
    fireEvent.change(screen.getByLabelText("Current title"), { target: { value: "Senior Accountant" } });

    next = failure("SERVICE_UNAVAILABLE", "The service is unavailable. Try again shortly.", undefined, 503);
    await client.refetchQueries({ queryKey: candidateKey(candidate.id) });

    expect(await screen.findByText("Showing the version loaded earlier.")).toBeTruthy();
    expect(screen.getByLabelText<HTMLInputElement>("Current title").value).toBe("Senior Accountant");
  });

  it("shows nationality, source and work authorization in words", async () => {
    await show(
      testCandidate({ nationality: "MY", source: "careers_page", workAuthorization: { status: "expat_eligible", basis: "rp_t" } }),
      RECRUITER,
    );
    expect(screen.getByText("Malaysia (MY)")).toBeTruthy();
    expect(screen.getByText("Careers page")).toBeTruthy();
    expect(screen.getByText("Expat, can work here: Residence Pass-Talent (RP-T)")).toBeTruthy();
  });
});

describe("possible duplicates", () => {
  it("warns about candidates with the same phone or a similar name", async () => {
    const similar = { items: [{ candidate: { id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Siti Aishah", email: null, phone: null, currentTitle: "Accountant", currentCompany: null, location: null, workAuthorizationStatus: null, updatedAt: "2026-11-01T00:00:00Z" }, reasons: ["similar_name"] }] };
    vi.spyOn(api, "GET").mockImplementation(((path: string) => {
      if (path === "/v1/candidates/{candidateId}/similar") return Promise.resolve(ok(similar));
      if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(testCandidate()));
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      return Promise.resolve(ok({ items: [] }));
    }));
    renderWithClient(<CandidateDetail id={testCandidate().id} permissions={RECRUITER} />);
    const warning = await screen.findByRole("region", { name: "Possible duplicates" });
    expect(warning.textContent).toContain("Siti Aishah");
    expect(warning.textContent).toContain("similar name; Accountant");
  });

  it("merges a duplicate into this candidate after confirming", async () => {
    const similar = { items: [{ candidate: { id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Siti Aishah", email: null, phone: null, currentTitle: null, currentCompany: null, location: null, workAuthorizationStatus: null, updatedAt: "2026-11-01T00:00:00Z" }, reasons: ["same_phone"] }] };
    vi.spyOn(api, "GET").mockImplementation(((path: string) => {
      if (path === "/v1/candidates/{candidateId}/similar") return Promise.resolve(ok(similar));
      if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(testCandidate()));
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      return Promise.resolve(ok({ items: [] }));
    }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(testCandidate({ version: 4 })));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<CandidateDetail id={testCandidate().id} permissions={[...RECRUITER, "candidates:merge"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Merge into this one" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ duplicateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", version: 3 });
  });
});
