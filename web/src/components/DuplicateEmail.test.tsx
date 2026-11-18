import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, ApiError } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate } from "../test-utils";
import { DuplicateEmail } from "./DuplicateEmail";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  location.hash = "";
});

const duplicate = (details: Record<string, unknown>) =>
  new ApiError({ error: { code: "CANDIDATE_DUPLICATE_EMAIL", message: "Another candidate already has this email.", requestId: "req_1", details } });
const siti = { existingCandidateId: "cand_1", existingCandidateName: "Siti Aisyah", existingOwnerName: "Aina" };

describe("duplicate email", () => {
  it("links to the candidate who has the email, naming their owner", () => {
    renderWithClient(<DuplicateEmail error={duplicate(siti)} />);
    expect(screen.getByRole("link", { name: "Siti Aisyah" }).getAttribute("href")).toBe("#/candidates/cand_1");
    expect(screen.getByText(/owned by Aina/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Merge/ }), "only for those who may merge").toBeNull();
  });

  it("says nothing more about a candidate the user may not see, or another error", () => {
    renderWithClient(<><DuplicateEmail error={duplicate({})} /><DuplicateEmail error={new Error("x")} /></>);
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.queryByText(/already has it/)).toBeNull();
  });

  it("merges this candidate into the one who has the email, then opens them", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...testCandidate(), id: "cand_1", version: 7 }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...testCandidate(), id: "cand_1", version: 8 }));
    renderWithClient(<DuplicateEmail error={duplicate(siti)} merge={{ id: "cand_new", name: "Siti A" }} />);
    fireEvent.click(screen.getByRole("button", { name: "Merge into Siti Aisyah" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/candidates/{candidateId}/merge");
    expect(sentBody(post)).toEqual({ duplicateId: "cand_new", version: 7 }); // kept: the one who had the email
    await waitFor(() => { expect(location.hash).toBe("#/candidates/cand_1"); });
  });
});
