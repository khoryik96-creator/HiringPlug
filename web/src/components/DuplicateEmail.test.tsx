import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { ApiError } from "../api/client";
import { DuplicateEmail } from "./DuplicateEmail";

afterEach(cleanup);

const duplicate = (details: Record<string, unknown>) =>
  new ApiError({ error: { code: "CANDIDATE_DUPLICATE_EMAIL", message: "Another candidate already has this email.", requestId: "req_1", details } });

describe("duplicate email", () => {
  it("links to the candidate who has the email, naming their owner", () => {
    render(<DuplicateEmail error={duplicate({ existingCandidateId: "cand_1", existingCandidateName: "Siti Aisyah", existingOwnerName: "Aina" })} />);
    expect(screen.getByRole("link", { name: "Siti Aisyah" }).getAttribute("href")).toBe("#/candidates/cand_1");
    expect(screen.getByText(/owned by Aina/)).toBeTruthy();
  });

  it("says nothing more about a candidate the user may not see, or another error", () => {
    const { container } = render(<><DuplicateEmail error={duplicate({})} /><DuplicateEmail error={new Error("x")} /></>);
    expect(container.textContent).toBe("");
  });
});
