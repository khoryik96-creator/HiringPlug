import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Application, type Permission } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ApplicationFit, fitBand, FitBadge, FitReasons, type Match } from "./Fit";
import { ratingBand } from "./FitStats";
import { PipelineBoard } from "./PipelineBoard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const match = (over: Partial<Match>): Match => ({
  id: "mat_1", applicationId: "appl_1", jobId: "jo_1", status: "done", score: 50, summary: "A partial fit.",
  reasons: [
    { requirement: "Degree in Accounting", verdict: "met", evidence: "Bachelor of Accounting, UiTM", note: "Accounting degree." },
    { requirement: "5 years in audit", verdict: "missing", evidence: null, note: null },
  ],
  dropped: 1, outdated: false, error: null, model: "deepseek-chat", createdAt: "2026-11-04T01:00:00Z", scoredAt: "2026-11-04T01:01:00Z",
  dimensions: [], strengths: [], gaps: [], recommendation: null, ...over,
});
const READ: Permission[] = ["matching:read", "pipeline:read"];
const SCORE: Permission[] = [...READ, "matching:create"];
const gets = (byPath: Record<string, unknown>) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(byPath[path]))) as never);

describe("fit stats (CV Studio's CV scoring)", () => {
  it("show the recommendation, the six ratings as bars with notes, and the AI's strengths and gaps", () => {
    expect([ratingBand(70), ratingBand(69), ratingBand(45), ratingBand(44)]).toEqual(["high", "mid", "mid", "low"]);
    renderWithClient(<FitReasons open match={match({
      recommendation: "consider",
      dimensions: [
        { name: "skills", score: 82, note: "Strong IFRS." },
        { name: "leadership", score: 30, note: null },
      ],
      strengths: ["Hands-on SAP"], gaps: ["Few audit years"],
    })} />);
    expect(screen.getByText("Consider with reservations").className).toBe("badge reco-consider");
    const ratings = within(screen.getByRole("list", { name: "Ratings" })).getAllByRole("listitem");
    expect(ratings.map((r) => r.textContent)).toEqual(["Technical / functional skills82/100Strong IFRS.", "Leadership & communication30/100"]);
    const skills = screen.getByRole("meter", { name: "Technical / functional skills" });
    expect(skills.getAttribute("aria-valuenow")).toBe("82");
    expect((skills.firstChild as HTMLElement).className).toBe("fit-bar-fill high");
    expect(screen.getByText("Hands-on SAP")).toBeTruthy();
    expect(screen.getByText("Few audit years")).toBeTruthy();
    expect(screen.getByText(/AI's opinion/)).toBeTruthy();
  });

  it("are left out for a score made before them", () => {
    renderWithClient(<FitReasons open match={match({})} />);
    expect(screen.queryByText(/Recommendation/)).toBeNull();
    expect(screen.queryByRole("list", { name: "Ratings" })).toBeNull();
    expect(screen.queryByText(/AI's opinion/)).toBeNull();
  });
});

describe("fit scores", () => {
  it("show the score in a colour by band, or why there is none", () => {
    expect([fitBand(80), fitBand(75), fitBand(50), fitBand(49)]).toEqual(["strong", "strong", "partial", "weak"]);
    renderWithClient(<><FitBadge match={match({ score: 80 })} /><FitBadge match={match({ status: "queued", score: null })} /></>);
    expect(screen.getByText("Fit 80%").className).toBe("badge fit-strong");
    expect(screen.getByRole("status").textContent).toBe("Scoring fit…");
  });

  it("explain each requirement with the job's and the CV's words", () => {
    renderWithClient(<FitReasons match={match({})} />);
    fireEvent.click(screen.getByText("Why 50%?"));
    const items = screen.getAllByRole("listitem");
    expect(items[0]?.textContent).toBe("Meets: “Degree in Accounting”CV: “Bachelor of Accounting, UiTM”Accounting degree.");
    expect(items[1]?.textContent).toBe("Not in the CV: “5 years in audit”");
    expect(screen.getByText(/1 requirement\(s\) it listed were left out/).textContent).toContain("by deepseek-chat");
    cleanup();
    renderWithClient(<FitReasons match={match({ status: "failed", score: null, error: "the AI provider did not answer in time" })} />);
    expect(screen.getByText("No fit score: the AI provider did not answer in time")).toBeTruthy();
  });

  it("are not shown, or fetched, without the permission or while matching is off", async () => {
    const get = gets({ "/v1/matches": { enabled: false, items: [] } });
    renderWithClient(<ApplicationFit applicationId="appl_1" permissions={["pipeline:read"]} />);
    expect(get).not.toHaveBeenCalled();
    cleanup();
    get.mockClear();
    renderWithClient(<ApplicationFit applicationId="appl_1" permissions={SCORE} />);
    await waitFor(() => { expect(get).toHaveBeenCalled(); });
    expect(screen.queryByRole("button", { name: "Score fit" })).toBeNull();
  });

  it("score one of a candidate's jobs, and again once scored", async () => {
    gets({ "/v1/matches": { enabled: true, items: [] } });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(match({ status: "queued", score: null })));
    renderWithClient(<ApplicationFit applicationId="appl_1" permissions={SCORE} />);
    fireEvent.click(await screen.findByRole("button", { name: "Score fit" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ applicationId: "appl_1" });

    cleanup();
    gets({ "/v1/matches": { enabled: true, items: [match({})] } });
    renderWithClient(<ApplicationFit applicationId="appl_1" permissions={READ} />);
    expect(await screen.findByText("Fit 50%")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Score again" })).toBeNull();
  });
});

const app = (id: string, name: string): Application => ({
  id, candidateId: `cand_${id}`, candidateName: name, stage: "screening", version: 1, jobId: "jo_1", jobTitle: "Auditor", companyId: "comp_1",
  companyName: "Maybank", candidateTitle: null, reason: null, reasonNote: null, channel: null,
  stageChangedAt: new Date().toISOString(), createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
});

describe("fit on a job's board", () => {
  it("shows each card's score, puts the best fits first, and scores everyone", async () => {
    gets({
      "/v1/jobs/{jobId}/applications": { items: [app("a1", "Ali"), app("a2", "Siti"), app("a3", "Wei")] },
      "/v1/matches": { enabled: true, items: [match({ applicationId: "a1", score: 40 }), match({ id: "mat_2", applicationId: "a2", score: 90 })] },
    });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ queued: 1, alreadyScored: 2, withoutCv: 1, consentWithdrawn: 1 }));
    renderWithClient(<PipelineBoard jobId="jo_1" canUpdate={false} permissions={SCORE} />);
    const column = screen.getByRole("list", { name: "Screening" });
    expect(await within(column).findByText("Fit 90%")).toBeTruthy();
    const names = () => within(column).getAllByRole("link").map((a) => a.textContent);
    expect(names()).toEqual(["Ali", "Siti", "Wei"]);
    fireEvent.click(screen.getByLabelText("Best fit first"));
    expect(names()).toEqual(["Siti", "Ali", "Wei"]); // unscored last
    fireEvent.click(screen.getByRole("button", { name: "Fit of Siti: why?" }));
    const panel = screen.getByRole("region", { name: "Fit of Siti" });
    expect(within(panel).getByText("Why 90%?")).toBeTruthy();
    expect(within(panel).getByText("Degree in Accounting", { exact: false })).toBeTruthy();
    fireEvent.click(within(panel).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("region", { name: "Fit of Siti" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Score candidates' fit" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ jobId: "jo_1", rescore: false });
    expect((await screen.findByText(/Scoring 1 candidate/)).textContent).toBe("Scoring 1 candidate(s)… 1 without a readable CV left out. 1 who withdrew consent left out.");
    fireEvent.click(screen.getByRole("button", { name: "Score all again" }));
    await waitFor(() => { expect(sentBody(post, 1)).toEqual({ jobId: "jo_1", rescore: true }); });
  });

  it("says when matching is off, and shows nothing without the permission", async () => {
    gets({ "/v1/jobs/{jobId}/applications": { items: [app("a1", "Ali")] }, "/v1/matches": { enabled: false, items: [] } });
    renderWithClient(<PipelineBoard jobId="jo_1" canUpdate={false} permissions={SCORE} />);
    expect(await screen.findByText(/Fit scores turn on once an AI provider is set up/)).toBeTruthy();
    cleanup();
    vi.restoreAllMocks();
    const get = gets({ "/v1/jobs/{jobId}/applications": { items: [app("a1", "Ali")] } });
    renderWithClient(<PipelineBoard jobId="jo_1" canUpdate={false} />);
    await screen.findByText("Ali");
    expect(get.mock.calls.map((c) => c[0])).toEqual(["/v1/jobs/{jobId}/applications"]);
  });
});

describe("an outdated score", () => {
  it("says the job changed since", () => {
    renderWithClient(<><FitBadge match={match({ outdated: true })} /><FitReasons match={match({ outdated: true })} open /></>);
    expect(screen.getByText("Fit 50% · outdated").getAttribute("title")).toContain("score it again");
    expect(screen.getByText(/changed after this score/)).toBeTruthy();
  });
});
