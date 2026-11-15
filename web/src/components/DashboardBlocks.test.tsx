import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { arrange, type Block, DashboardBlocks, moved } from "./DashboardBlocks";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const blocks: Block[] = [
  { id: "numbers", label: "the numbers", node: <p>Numbers</p> },
  { id: "needsYou", label: "Needs you today", node: <p>Needs</p> },
  { id: "openJobs", label: "Open jobs", node: <p>Jobs</p> },
];
const texts = () => [...document.querySelectorAll(".dash-block p")].map((p) => p.textContent);

describe("arranging the dashboard", () => {
  it("keeps a saved order, and puts blocks it does not name after it", () => {
    expect(arrange(blocks, null).map((b) => b.id)).toEqual(["numbers", "needsYou", "openJobs"]);
    expect(arrange(blocks, ["openJobs", "tasks"]).map((b) => b.id)).toEqual(["openJobs", "numbers", "needsYou"]); // tasks: not shown here
    expect(moved(["numbers", "needsYou", "openJobs"], "openJobs", 0)).toEqual(["openJobs", "numbers", "needsYou"]);
    expect(moved(["numbers", "needsYou", "openJobs"], "numbers", 9)).toEqual(["needsYou", "openJobs", "numbers"]);
  });

  it("shows the saved order, moves a block with its handle and saves the new order", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ candidateColumns: null, dashboardBlocks: ["needsYou", "numbers", "openJobs"] }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ candidateColumns: null, dashboardBlocks: ["needsYou", "openJobs", "numbers"] }));
    renderWithClient(<DashboardBlocks blocks={blocks} />);
    await waitFor(() => { expect(texts()).toEqual(["Needs", "Numbers", "Jobs"]); });
    fireEvent.keyDown(screen.getByRole("button", { name: "Move the numbers (arrow keys)" }), { key: "ArrowDown" });
    await waitFor(() => { expect(texts()).toEqual(["Needs", "Jobs", "Numbers"]); }); // before the server answers
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ dashboardBlocks: ["needsYou", "openJobs", "numbers"] }); // only the order: the columns stay
    fireEvent.click(screen.getByRole("button", { name: "Reset order" }));
    await waitFor(() => { expect(((put.mock.calls as unknown[][]).at(-1)?.[1] as { body: unknown }).body).toEqual({ dashboardBlocks: null }); });
  });

  it("moves a block by dragging its handle onto another", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ candidateColumns: null, dashboardBlocks: null }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({}));
    renderWithClient(<DashboardBlocks blocks={blocks} />);
    await waitFor(() => { expect(texts()).toEqual(["Numbers", "Needs", "Jobs"]); });
    const target = screen.getByText("Numbers").closest(".dash-block") as HTMLElement;
    const handle = screen.getByRole("button", { name: "Move Open jobs (arrow keys)" });
    fireEvent.dragStart(handle, { dataTransfer: { setData: vi.fn(), setDragImage: vi.fn(), effectAllowed: "" } });
    fireEvent.dragOver(target);
    fireEvent.drop(target);
    await waitFor(() => { expect(sentBody(put)).toEqual({ dashboardBlocks: ["openJobs", "numbers", "needsYou"] }); });
  });
});
