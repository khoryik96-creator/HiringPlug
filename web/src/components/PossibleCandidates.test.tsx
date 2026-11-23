import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Permission } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Highlighted, PossibleCandidates, tidySnippet } from "./PossibleCandidates";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const SEE: Permission[] = ["matching:read", "candidates:read", "documents:read", "pipeline:read"];

describe("possible candidates", () => {
  it("are not looked for without the permissions", () => {
    const get = vi.spyOn(api, "GET");
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={["matching:read"]} />);
    expect(screen.queryByRole("group", { name: "Possible candidates" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("show where the CV has the job's words, and add someone to the job", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ candidateId: "cand_1", candidateName: "Aina", currentTitle: "Auditor", rank: 0.6,
      snippet: "Aina aina@x.com +60 12 345 6789 statutory [[audit]] of listed [[groups]] … [[Audit]] lead" }], needsText: false }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={[...SEE, "pipeline:update"]} />);
    expect(await screen.findByText("Aina")).toBeTruthy();
    // Folded under the pipeline, with how many there are.
    expect(screen.getByRole("group", { name: "Possible candidates" }).querySelector("summary")?.textContent).toBe("Possible candidates1");
    expect(screen.getByLabelText("Matched words").textContent).toBe("2 matched:auditgroups"); // once each
    const excerpt = document.querySelector(".suggestion-snippet");
    expect(excerpt?.textContent).toBe("Aina statutory audit of listed groups … Audit lead"); // contact details are on the profile
    expect(excerpt?.querySelector("mark")?.textContent).toBe("audit");
    fireEvent.click(screen.getByRole("button", { name: "Add to job" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ candidateId: "cand_1" });
  });

  it("say why someone found by meaning matches, though their CV shares none of the job's words", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({
      items: [{ candidateId: "cand_2", candidateName: "Nurul", currentTitle: "Talent Acquisition Partner", rank: 0.9, snippet: "",
        why: { score: 0.91, strength: "strong", title: "Talent Acquisition Partner", titleWhen: "now", family: "recruitment", level: "Executive",
          passage: "Hired 120 engineers a year", words: [] } }],
      meaning: { status: "ready", understood: { role: "recruiter", family: "recruitment", describes: false } }, needsText: false,
    }));
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={SEE} />);
    expect(await screen.findByText("Nurul")).toBeTruthy();
    expect(document.querySelector(".why-line")?.textContent)
      .toBe("Strong matchas Talent Acquisition Partner (now) · recruitmentHired 120 engineers a year"); // no level: the job names none
    expect(document.querySelector(".suggestion-snippet")).toBeNull();
    expect(screen.queryByLabelText("Matched words")).toBeNull();
  });

  it("quote a CV once: where it matches by meaning, with the job's words it has as chips", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({
      items: [{ candidateId: "cand_3", candidateName: "Priya", currentTitle: "HR Executive (Recruitment)", rank: 0.8,
        snippet: "HR Executive ([[Recruitment]]) … walk-in [[interviews]]",
        why: { score: 0.6, strength: "good", title: "HR Executive (Recruitment)", titleWhen: "now", family: "recruitment", level: "Executive",
          passage: "Recruitment of operators and engineers: walk-in interviews", words: ["Recruitment", "interviews"] } }],
      meaning: { status: "ready", understood: { role: "talent acquisition manager", family: "recruitment", level: 5, levelName: "Manager", describes: false } },
      needsText: false,
    }));
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={SEE} />);
    expect(await screen.findByText("Priya")).toBeTruthy();
    expect(document.querySelector(".why-line")?.textContent) // the job names a level: theirs is shown
      .toBe("Good matchas HR Executive (Recruitment) (now) · recruitment · level: executiveRecruitment of operators and engineers: walk-in interviews");
    expect(screen.getByLabelText("Matched words").textContent).toBe("2 matched:Recruitmentinterviews");
    expect(document.querySelector(".suggestion-snippet")).toBeNull();
  });

  it("say the job is being read by meaning, and not that no one matches meanwhile", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], meaning: { status: "reading" }, needsText: false }));
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={SEE} />);
    expect((await screen.findByRole("status")).textContent).toContain("Reading the job the way a recruiter would");
    expect(screen.queryByText("No one matches yet.")).toBeNull();
  });

  it("ask for the job's text when there is none", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], needsText: true })); // an answer, not an error the browser logs
    renderWithClient(<PossibleCandidates jobId="jo_1" permissions={SEE} />);
    expect(await screen.findByText(/Add a description or an ad/)).toBeTruthy();
    expect(screen.queryByText("No one matches yet.")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(document.querySelector(".count-chip")).toBeNull();
  });

  it("highlight only what is marked", () => {
    renderWithClient(<p data-testid="t"><Highlighted text="no marks [[here]] and [[there]]." /></p>);
    expect(screen.getByTestId("t").innerHTML).toBe("no marks <mark>here</mark> and <mark>there</mark>.");
  });

  it("hide contact details from snippets, but keep dates", () => {
    expect(tidySnippet("Call +60 12-345 6789 or siti@maju.test • Accountant [[2018]] - 2020, 2015 - 2018 2018 - 2020"))
      .toBe("Call or · Accountant [[2018]] - 2020, 2015 - 2018 2018 - 2020");
  });
});
