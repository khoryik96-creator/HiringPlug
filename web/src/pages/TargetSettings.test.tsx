import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { invalidField, ok, renderWithClient } from "../test-utils";
import { draftsOf, TargetSettings, targetsOf } from "./TargetSettings";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const saved: Schemas["ReportSettings"] = {
  targets: [{ userId: null, period: "week", goal: { calls: 20, cvsSent: 5, interviews: null, placements: null } }],
  stageWeights: [
    { stage: "submitted", percent: 10, monthsAhead: 2 }, { stage: "interview", percent: 30, monthsAhead: 1 },
    { stage: "offer", percent: 70, monthsAhead: 0 }, { stage: "placed", percent: 100, monthsAhead: 0 },
  ],
};
const team: Schemas["ActivityReport"] = {
  period: "week", from: "2026-11-16", to: "2026-11-22", team: true,
  rows: [{ userId: "usr_a", userName: "Aina", done: { calls: 0, cvsSent: 0, interviews: 0, placements: 0, categories: {} }, target: null }],
};
const categories: Schemas["StageCategories"] = {
  version: "0",
  items: [
    { key: "applied", label: "Applied", kind: "applied" }, { key: "submitted", label: "Submitted", kind: "submitted" },
    { key: "interview", label: "1st Interview", kind: "interview" }, { key: "interview_final", label: "Final Interview", kind: "interview" },
    { key: "placed", label: "Placed", kind: "placed" },
  ],
};
const reads = () => vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
  path === "/v1/reports/settings" ? saved : path === "/v1/stage-categories" ? categories : team))) as never);

describe("targets and forecast settings", () => {
  it("keeps only the targets with a number", () => {
    const drafts = draftsOf(saved.targets);
    expect(drafts["|week"]).toEqual({ calls: "20", cvsSent: "5", interviews: "", placements: "" });
    expect(targetsOf({ ...drafts, "usr_a|month": { calls: "", cvsSent: "", interviews: "", placements: "" } })).toEqual(saved.targets);
  });

  it("saves a recruiter's own target and the stage weights", async () => {
    reads();
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(saved));
    renderWithClient(<TargetSettings />);
    expect((await screen.findByLabelText<HTMLInputElement>("Everyone (unless set below): Calls per week")).value).toBe("20");
    fireEvent.click(screen.getByRole("button", { name: "Per month" }));
    fireEvent.change(await screen.findByLabelText("Aina: Placements per month"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Interview: percent of the fee"), { target: { value: "40" } });
    fireEvent.change(screen.getByLabelText("Offer: month the fee is expected"), { target: { value: "1" } });
    fireEvent.click(screen.getByRole("button", { name: "Save targets and weights" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    const body = (put.mock.calls[0]?.[1] as unknown as { body: Schemas["ReportSettings"] }).body;
    expect(body.targets).toEqual([
      saved.targets[0],
      { userId: "usr_a", period: "month", goal: { calls: null, cvsSent: null, interviews: null, placements: 2 } },
    ]);
    expect(body.stageWeights[1]).toEqual({ stage: "interview", percent: 40, monthsAhead: 1 });
    expect(body.stageWeights[2]).toEqual({ stage: "offer", percent: 70, monthsAhead: 1 });
    expect(await screen.findByText("Saved.")).toBeTruthy();
  });

  it("sets targets for the company's stage categories (not those CVs sent and Placements count)", async () => {
    reads();
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(saved));
    renderWithClient(<TargetSettings />);
    fireEvent.change(await screen.findByLabelText("Everyone (unless set below): 1st Interview per week"), { target: { value: "6" } });
    fireEvent.change(screen.getByLabelText("Aina: Final Interview per week"), { target: { value: "2" } });
    expect(screen.queryByLabelText("Aina: Submitted per week")).toBeNull();
    expect(screen.queryByLabelText("Aina: Placed per week")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save targets and weights" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    const body = (put.mock.calls[0]?.[1] as unknown as { body: Schemas["ReportSettings"] }).body;
    expect(body.targets).toEqual([
      { ...saved.targets[0], goal: { ...saved.targets[0]?.goal, categories: { interview: 6 } } },
      { userId: "usr_a", period: "week", goal: { calls: null, cvsSent: null, interviews: null, placements: null, categories: { interview_final: 2 } } },
    ]);
  });

  it("says whose target a problem is about", async () => {
    reads();
    vi.spyOn(api, "PUT").mockResolvedValue(invalidField("targets.0.goal.calls", "between 0 and 100000"));
    renderWithClient(<TargetSettings />);
    fireEvent.change(await screen.findByLabelText("Everyone (unless set below): Calls per week"), { target: { value: "30" } });
    await screen.findByLabelText("Aina: Calls per week");
    fireEvent.click(screen.getByRole("button", { name: "Save targets and weights" }));
    expect(await screen.findByText("Everyone (unless set below), per week: between 0 and 100000")).toBeTruthy();
  });
});
