import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { byDay, dayLabel, StageMoves } from "./StageMoves";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const move = (over: Partial<Schemas["StageMove"]>): Schemas["StageMove"] => ({
  id: "stg_1", applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah", jobId: "job_1", jobTitle: "Auditor",
  companyName: "Maybank", fromStage: "applied", toStage: "screening", changedAt: new Date().toISOString(), changedByName: "Aina", ...over,
});

describe("stage move timeline", () => {
  it("names days the way people say them", () => {
    const now = new Date(2026, 10, 5, 15, 0);
    expect(dayLabel(new Date(2026, 10, 5, 0, 5), now)).toBe("Today");
    expect(dayLabel(new Date(2026, 10, 4, 23, 59), now)).toBe("Yesterday");
    expect(dayLabel(new Date(2026, 10, 1, 9, 0), now)).not.toMatch(/Today|Yesterday|2026/);
    expect(dayLabel(new Date(2025, 10, 1, 9, 0), now)).toMatch(/2025/);
    const moves = [
      move({ id: "a", changedAt: new Date(2026, 10, 5, 10).toISOString() }),
      move({ id: "b", changedAt: new Date(2026, 10, 5, 9).toISOString() }),
      move({ id: "c", changedAt: new Date(2026, 10, 3, 9).toISOString() }),
    ];
    expect(byDay(moves, now).map(([day, items]) => [day, items.map((m) => m.id)])).toEqual([
      ["Today", ["a", "b"]], [dayLabel(new Date(2026, 10, 3), now), ["c"]],
    ]);
  });

  it("shows who moved whom, from which stage to which, on which job", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      move({}),
      move({ id: "stg_2", candidateId: "cand_2", candidateName: "Bala", fromStage: null, toStage: "applied", changedByName: null }),
    ] }));
    renderWithClient(<StageMoves />);
    const panel = await screen.findByRole("region", { name: "Recent moves" });
    const [moved, added] = await within(panel).findAllByRole("listitem");
    expect(moved?.textContent).toMatch(/Siti Aisyah.*Applied → .*Screening.*on Auditor \(Maybank\) · by Aina/);
    expect(within(moved as HTMLElement).getByRole("link", { name: "Siti Aisyah" }).getAttribute("href")).toBe("#/candidates/cand_1");
    expect(added?.textContent).toMatch(/Bala added at Applied/);
    expect(added?.textContent).not.toContain("by");
    expect(get).toHaveBeenCalledWith("/v1/pipeline/moves", { params: { query: { candidateId: undefined, limit: 30 } } });
  });

  it("on a candidate's page, lists only theirs without repeating their name", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [move({})] }));
    renderWithClient(<StageMoves candidateId="cand_1" title="Stage history" limit={50} />);
    const panel = await screen.findByRole("region", { name: "Stage history" });
    await within(panel).findByText("Auditor");
    expect(within(panel).queryByRole("link", { name: "Siti Aisyah" })).toBeNull();
    expect(get).toHaveBeenCalledWith("/v1/pipeline/moves", { params: { query: { candidateId: "cand_1", limit: 50 } } });
  });
});
