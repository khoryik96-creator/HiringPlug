import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Application } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { PipelineBoard } from "./PipelineBoard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const base: Omit<Application, "id" | "candidateId" | "candidateName" | "stage"> = {
  version: 3, jobId: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", jobTitle: "Auditor", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7",
  companyName: "Maybank", candidateTitle: "Accountant", reason: null, reasonNote: null, channel: null,
  stageChangedAt: new Date(Date.now() - 3 * 86_400_000).toISOString(), createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};
const siti: Application = { ...base, id: "appl_01jbz6w0m0k8q9x2r3s4t5v6w1", candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w1", candidateName: "Siti Aisyah", stage: "screening" };
const wei: Application = { ...base, id: "appl_01jbz6w0m0k8q9x2r3s4t5v6w2", candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w2", candidateName: "Wei Tan", stage: "rejected", reason: "salary_mismatch", reasonNote: "Wants 15k" };

async function showBoard(canUpdate = true) {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [siti, wei] }));
  renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate={canUpdate} />);
  await screen.findByText("Siti Aisyah");
}

describe("pipeline board", () => {
  it("shows each candidate in their stage, and closed ones with the reason", async () => {
    await showBoard();
    expect(within(screen.getByRole("list", { name: "Screening" })).getByText("Siti Aisyah")).toBeTruthy();
    expect(within(screen.getByRole("list", { name: "Screening" })).getByText("3 days in screening")).toBeTruthy();
    expect(screen.getByText("Rejected or withdrawn").closest("summary")?.textContent).toBe("Rejected or withdrawn 1");
    expect(screen.getByText(/Salary does not match: Wants 15k/)).toBeTruthy();
  });

  it("moves a card with its menu, sending the version it showed", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...siti, stage: "interview", version: 4 }));
    await showBoard();
    fireEvent.change(screen.getByRole("combobox", { name: "Move Siti Aisyah on Auditor" }), { target: { value: "interview" } });
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 3, stage: "interview" });
  });

  it("asks why before rejecting", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...siti, stage: "rejected", reason: "not_qualified", version: 4 }));
    await showBoard();
    fireEvent.change(screen.getByRole("combobox", { name: "Move Siti Aisyah on Auditor" }), { target: { value: "rejected" } });
    const form = screen.getByRole("form", { name: "Rejected: why?" });
    expect(within(form).getByRole<HTMLButtonElement>("button", { name: "Rejected" }).disabled).toBe(true);
    fireEvent.change(within(form).getByLabelText("Reason"), { target: { value: "not_qualified" } });
    fireEvent.change(within(form).getByLabelText("Note (optional)"), { target: { value: " No CPA " } });
    fireEvent.click(within(form).getByRole("button", { name: "Rejected" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 3, stage: "rejected", reason: "not_qualified", reasonNote: "No CPA" });
  });

  it("moves a card dragged onto another column", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...siti, stage: "offer", version: 4 }));
    await showBoard();
    const card = screen.getByText("Siti Aisyah").closest(".card");
    if (!card) throw new Error("no card");
    const dataTransfer = { setData: vi.fn(), getData: vi.fn() };
    fireEvent.dragStart(card, { dataTransfer });
    const offer = screen.getByRole("list", { name: "Offer" });
    fireEvent.dragOver(offer, { dataTransfer });
    fireEvent.drop(offer, { dataTransfer });
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 3, stage: "offer" });
  });

  it("says when someone else moved the card first", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(failure("CONFLICT_STALE_VERSION", "This application was changed by someone else. Reload it and try again.", undefined, 409));
    await showBoard();
    fireEvent.change(screen.getByRole("combobox", { name: "Move Siti Aisyah on Auditor" }), { target: { value: "offer" } });
    expect(await screen.findByRole("button", { name: "Reload the board" })).toBeTruthy();
  });

  it("is read-only without pipeline:update", async () => {
    await showBoard(false);
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.getByText("Siti Aisyah").closest(".card")?.getAttribute("draggable")).toBe("false");
  });
});
