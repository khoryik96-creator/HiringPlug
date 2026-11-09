import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Permission } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { Highlighted, PossibleCandidates } from "./PossibleCandidates";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const SEE: Permission[] = ["matching:read", "candidates:read", "documents:read", "pipeline:read"];

describe("possible candidates", () => {
  it("are not looked for without the permissions", () => {
    const get = vi.spyOn(api, "GET");
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={["matching:read"]} />);
    expect(screen.queryByRole("region", { name: "Possible candidates" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("show where the CV has the job's words, and add someone to the job", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ candidateId: "cand_1", candidateName: "Aina", currentTitle: "Auditor", rank: 0.6,
      snippet: "Aina aina@x.com +60 12 345 6789 statutory [[audit]] of listed [[groups]] … [[Audit]] lead" }] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={[...SEE, "pipeline:update"]} />);
    expect(await screen.findByText("Aina")).toBeTruthy();
    expect(screen.getByLabelText("Matched words").textContent).toBe("2 matched:auditgroups"); // once each
    const excerpt = document.querySelector(".suggestion-snippet");
    expect(excerpt?.textContent).toBe("Aina statutory audit of listed groups … Audit lead"); // contact details are on the profile
    expect(excerpt?.querySelector("mark")?.textContent).toBe("audit");
    fireEvent.click(screen.getByRole("button", { name: "+ Add to job" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ candidateId: "cand_1" });
  });

  it("ask for the job's text when there is none", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(failure("MATCH_NEEDS_TEXT", "No text.", undefined, 409));
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={SEE} />);
    expect(await screen.findByText(/Add a description or an ad/)).toBeTruthy();
  });

  it("highlight only what is marked", () => {
    renderWithClient(<p data-testid="t"><Highlighted text="no marks [[here]] and [[there]]." /></p>);
    expect(screen.getByTestId("t").innerHTML).toBe("no marks <mark>here</mark> and <mark>there</mark>.");
  });
});
