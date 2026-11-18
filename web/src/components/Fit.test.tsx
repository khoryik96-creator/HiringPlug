import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi, beforeEach } from "vitest";

import { api, type Application, type Permission } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ApplicationFit, fitBand, FitBadge, FitReasons, type Match } from "./Fit";
import { ratingBand } from "./FitStats";
import { PipelineBoard } from "./PipelineBoard";

// About the board (the pipeline opens on Steps otherwise).
beforeEach(() => { localStorage.setItem("hp.pipelineView", "board"); });

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
    expect(screen.getByText(/^Scored /).textContent).toContain("by deepseek-chat");
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
  companyName: "Maybank", candidateTitle: null, candidateAvailability: null, reason: null, reasonNote: null, channel: null,
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
    fireEvent.change(screen.getByRole("combobox", { name: "Order" }), { target: { value: "fit" } });
    expect(names()).toEqual(["Siti", "Ali", "Wei"]); // unscored last
    // The Steps view follows it too.
    fireEvent.click(screen.getByRole("button", { name: "Steps" }));
    fireEvent.click(within(screen.getByRole("tablist", { name: "Stages" })).getByRole("tab", { name: /All/ }));
    expect(within(screen.getByRole("list", { name: "Everyone" })).getAllByRole("link").map((a) => a.textContent)).toEqual(["Siti", "Ali", "Wei"]);
    fireEvent.click(screen.getByRole("button", { name: "Board" }));
    fireEvent.click(screen.getByRole("button", { name: "Fit of Siti: why?" }));
    const panel = screen.getByRole("region", { name: "Fit of Siti" });
    expect(within(panel).getByText("Why 90%?")).toBeTruthy();
    expect(within(panel).getByText("Degree in Accounting", { exact: false })).toBeTruthy();
    fireEvent.click(within(panel).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("region", { name: "Fit of Siti" })).toBeNull();

    expect(screen.queryByRole("button", { name: "Score all again" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Score" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ jobId: "jo_1", rescore: false });
    expect((await screen.findByText(/Scoring 1/)).textContent).toBe("Scoring 1… 1 without a readable CV left out. 1 who withdrew consent left out.");
  });

  it("scores the chosen people again, and says when everyone is scored already", async () => {
    gets({
      "/v1/jobs/{jobId}/applications": { items: [app("a1", "Ali"), app("a2", "Siti")] },
      "/v1/matches": { enabled: true, items: [match({ applicationId: "a1", score: 40 }), match({ id: "mat_2", applicationId: "a2", score: 90 })] },
    });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ queued: 0, alreadyScored: 2, withoutCv: 0, consentWithdrawn: 0 }));
    renderWithClient(<PipelineBoard jobId="jo_1" canUpdate={false} permissions={[...SCORE, "emails:send"]} />);
    await screen.findByText("Fit 90%");
    fireEvent.click(screen.getByRole("button", { name: "Score" }));
    expect(await screen.findByText("Everyone is scored. Choose people to score them again.")).toBeTruthy();
    expect(sentBody(post)).toEqual({ jobId: "jo_1", rescore: false });

    const siti = screen.getByRole("link", { name: "Siti" });
    fireEvent.click(siti, { detail: 1 }); // a click on the name chooses them
    fireEvent.click(screen.getByRole("button", { name: "Score 1" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(post.mock.calls[1]?.[0]).toBe("/v1/matches");
    expect(sentBody(post, 1)).toEqual({ applicationId: "a2" });
    expect(await screen.findByText("Scoring 1…")).toBeTruthy();
  });

  it("scores many chosen people a few at a time", async () => {
    const people = ["Ali", "Siti", "Wei", "Mei", "Raj", "Tan"];
    gets({
      "/v1/jobs/{jobId}/applications": { items: people.map((name, i) => app(`a${String(i)}`, name)) },
      "/v1/matches": { enabled: true, items: [] },
    });
    let open = 0, most = 0;
    const post = vi.spyOn(api, "POST").mockImplementation((async () => {
      open++; most = Math.max(most, open);
      await new Promise((resolve) => setTimeout(resolve, 5));
      open--;
      return ok({ queued: 1 });
    }) as never);
    renderWithClient(<PipelineBoard jobId="jo_1" canUpdate={false} permissions={[...SCORE, "emails:send"]} />);
    for (const name of people) fireEvent.click(await screen.findByRole("link", { name }), { detail: 1 });
    fireEvent.click(screen.getByRole("button", { name: "Score 6" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(6); });
    expect(await screen.findByText("Scoring 6…")).toBeTruthy();
    expect(most).toBe(4);
  });

  it("says when matching is off, and shows nothing without the permission", async () => {
    gets({ "/v1/jobs/{jobId}/applications": { items: [app("a1", "Ali")] }, "/v1/matches": { enabled: false, items: [] } });
    renderWithClient(<PipelineBoard jobId="jo_1" canUpdate={false} permissions={SCORE} />);
    await screen.findByText("Ali");
    expect(screen.queryByText(/Fit/)).toBeNull(); // off: no fit scores, and nothing said about it
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

  it("is short on a board line, so the name keeps its room, and still says outdated", () => {
    renderWithClient(<FitBadge match={match({ outdated: true })} compact />);
    const badge = document.querySelector(".badge") as HTMLElement;
    expect(badge.textContent).toBe("Fit 50% ↻ · outdated");
    expect(badge.querySelector("[aria-hidden=true]")?.textContent).toBe(" ↻");
    expect(badge.querySelector(".visually-hidden")?.textContent).toBe(" · outdated");
    expect(badge.getAttribute("title")).toContain("score it again");
  });
});
