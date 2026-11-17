import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Application } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { PipelineBoard } from "./PipelineBoard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
beforeEach(() => { localStorage.removeItem("hp.pipelineView"); });

const ago = (d: number) => new Date(Date.now() - d * 86_400_000).toISOString();
const app = (n: number, stage: Application["stage"], days: number): Application => ({
  id: `appl_${String(n)}`, version: 2, jobId: "jo_1", jobTitle: "Risk Analyst", companyId: "comp_1", companyName: "Contoh", candidateId: `cand_${String(n)}`,
  candidateName: `Person ${String(n)}`, candidateTitle: "Analyst", stage, reason: null, reasonNote: null, channel: null,
  stageChangedAt: ago(days), createdAt: ago(30), updatedAt: ago(1),
});

function show(items: Application[]) {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items }));
  renderWithClient(<PipelineBoard jobId="jo_1" canUpdate permissions={["pipeline:read", "pipeline:update"]} />);
}

describe("pipeline steps", () => {
  it("opens on Steps at the stage where someone waits longest, with the button that moves them on", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...app(2, "interview", 0), version: 3 }));
    show([app(1, "applied", 1), app(4, "screening", 9), app(2, "submitted", 18), app(5, "submitted", 8), app(3, "submitted", 2)]);
    const stages = await screen.findByRole("tablist", { name: "Stages" });
    const submitted = within(stages).getByRole("tab", { name: /Submitted/ });
    expect(submitted.getAttribute("aria-selected")).toBe("true");
    expect(submitted.textContent).toContain("2 waiting 7+ days"); // more than Screening's 1, so it opens here
    const rows = within(screen.getByRole("list", { name: "Submitted" })).getAllByRole("listitem");
    expect(rows.map((r) => within(r).getByRole("link").textContent)).toEqual(["Person 2", "Person 5", "Person 3"]); // longest waiting first
    fireEvent.click(within(rows[0] as HTMLElement).getByRole("button", { name: "Interview" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 2, stage: "interview" });

    fireEvent.click(within(stages).getByRole("tab", { name: /All/ }));
    expect(within(screen.getByRole("list", { name: "Everyone" })).getAllByRole("listitem")).toHaveLength(5);
  });

  it("keeps the stage it opened at when people move on (it does not jump)", async () => {
    let moved = false;
    vi.spyOn(api, "GET").mockImplementation((() => Promise.resolve(ok({ items: moved
      ? [app(4, "screening", 9), app(2, "interview", 0), app(5, "submitted", 8)]
      : [app(4, "screening", 9), app(2, "submitted", 18), app(5, "submitted", 8)] }))) as never);
    vi.spyOn(api, "POST").mockImplementation((() => { moved = true; return Promise.resolve(ok({ ...app(2, "interview", 0), version: 3 })); }) as never);
    renderWithClient(<PipelineBoard jobId="jo_1" canUpdate permissions={["pipeline:read", "pipeline:update"]} />);
    const stages = await screen.findByRole("tablist", { name: "Stages" });
    expect(within(stages).getByRole("tab", { name: /Submitted/ }).getAttribute("aria-selected")).toBe("true");
    fireEvent.click(within(screen.getByRole("list", { name: "Submitted" })).getAllByRole("button", { name: "Interview" })[0] as HTMLElement);
    await waitFor(() => { expect(within(screen.getByRole("list", { name: "Submitted" })).getAllByRole("listitem")).toHaveLength(1); });
    // Now Screening and Submitted each have one waiting: it stays on Submitted.
    expect(within(stages).getByRole("tab", { name: /Submitted/ }).getAttribute("aria-selected")).toBe("true");
  });

  it("chooses a person in Steps with Space on their name, as on the board", async () => {
    show([app(1, "applied", 1)]);
    const name = await screen.findByRole("link", { name: "Person 1" });
    fireEvent.keyDown(name, { key: " " });
    expect(screen.getByRole("status").textContent).toContain("1 chosen");
  });

  it("switches to the board and remembers it; a crowded column shows 8 and folds the rest", async () => {
    show(Array.from({ length: 12 }, (_, i) => app(i + 1, "submitted", 12 - i)));
    fireEvent.click(await screen.findByRole("button", { name: "Board" }));
    expect(localStorage.getItem("hp.pipelineView")).toBe("board");
    const column = screen.getByRole("list", { name: "Submitted" });
    expect(within(column).getAllByRole("listitem")).toHaveLength(8);
    expect(within(column).getAllByRole("link")[0]?.textContent).toBe("Person 1"); // 12 days: longest first
    fireEvent.click(within(column).getByRole("button", { name: "+4 more" }));
    expect(within(column).getAllByRole("listitem")).toHaveLength(12);
  });
});
