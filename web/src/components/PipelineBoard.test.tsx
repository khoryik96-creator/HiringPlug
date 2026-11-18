import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Application } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { PipelineBoard } from "./PipelineBoard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
// These are about the board; the Steps view has its own tests (PipelineViews.test.tsx).
beforeEach(() => { localStorage.setItem("hp.pipelineView", "board"); });

const base: Omit<Application, "id" | "candidateId" | "candidateName" | "stage"> = {
  version: 3, jobId: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", jobTitle: "Auditor", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7",
  companyName: "Maybank", candidateTitle: "Accountant", candidateAvailability: null, reason: null, reasonNote: null, channel: null,
  stageChangedAt: new Date(Date.now() - 3 * 86_400_000).toISOString(), createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};
const siti: Application = { ...base, id: "appl_01jbz6w0m0k8q9x2r3s4t5v6w1", candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w1", candidateName: "Siti Aisyah", stage: "screening" };
const wei: Application = { ...base, id: "appl_01jbz6w0m0k8q9x2r3s4t5v6w2", candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w2", candidateName: "Wei Tan", stage: "rejected", reason: "salary_mismatch", reasonNote: "Wants 15k" };

/** Clicking a card (not its name) ticks it. */
/** A candidate's card on the board, by their name. */
const cardOf = (name: string) => { const card = screen.getByRole("link", { name }).closest<HTMLElement>(".board-line"); if (!card) throw new Error("no card"); return card; };
const choose = (name: string) => { fireEvent.click(cardOf(name)); };
const moveChosen = (stage: string) => { fireEvent.change(screen.getByRole("combobox", { name: "Move the chosen candidates" }), { target: { value: stage } }); };

async function showBoard(canUpdate = true) {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [siti, wei] }));
  renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate={canUpdate} />);
  await screen.findByRole("link", { name: "Siti Aisyah" });
}

describe("pipeline board", () => {
  it("marks the stage it was opened at from the dashboard, for a moment", async () => {
    window.location.hash = `#/jobs/${base.jobId}?stage=screening`;
    try {
      await showBoard();
      expect(screen.getByRole("list", { name: "Screening" }).className).toContain("spotlight");
      expect(screen.getByRole("list", { name: "Applied" }).className).not.toContain("spotlight");
      await waitFor(() => { expect(screen.getByRole("list", { name: "Screening" }).className).not.toContain("spotlight"); }, { timeout: 5000 });
    } finally {
      window.location.hash = "";
    }
  });

  it("shows each candidate in their stage, and closed ones with the reason", async () => {
    await showBoard();
    expect(within(screen.getByRole("list", { name: "Screening" })).getByRole("link", { name: "Siti Aisyah" })).toBeTruthy();
    expect(within(screen.getByRole("list", { name: "Screening" })).getByText("3d")).toBeTruthy();
    expect(screen.getByText("Rejected or withdrawn").closest("summary")?.textContent).toBe("Rejected or withdrawn 1");
    expect(screen.getByText(/Salary does not match: Wants 15k/)).toBeTruthy();
  });

  it("moves a chosen card from the bar, sending the version it showed", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...siti, stage: "interview", version: 4 }));
    await showBoard();
    expect(screen.queryByRole("combobox", { name: "Move the chosen candidates" })).toBeNull(); // nothing on the cards themselves
    choose("Siti Aisyah");
    expect(cardOf("Siti Aisyah").className).toContain("picked");
    moveChosen("interview");
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 3, stage: "interview" });
  });

  it("asks why before rejecting", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...siti, stage: "rejected", reason: "not_qualified", version: 4 }));
    await showBoard();
    choose("Siti Aisyah");
    moveChosen("rejected");
    const form = screen.getByRole("form", { name: "Rejected: why?" });
    fireEvent.click(within(form).getByRole("button", { name: "Rejected 1" }));
    expect(post).not.toHaveBeenCalled();
    expect(within(form).getByLabelText<HTMLSelectElement>("Reason").validity.valueMissing).toBe(true);
    fireEvent.change(within(form).getByLabelText("Reason"), { target: { value: "not_qualified" } });
    fireEvent.change(within(form).getByLabelText("Note (optional)"), { target: { value: " No CPA " } });
    fireEvent.click(within(form).getByRole("button", { name: "Rejected 1" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 3, stage: "rejected", reason: "not_qualified", reasonNote: "No CPA" });
  });

  it("moves a card dragged onto another column", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...siti, stage: "offer", version: 4 }));
    await showBoard();
    const card = cardOf("Siti Aisyah");
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
    choose("Siti Aisyah");
    moveChosen("offer");
    expect(await screen.findByRole("button", { name: "Reload the board" })).toBeTruthy();
  });

  it("asks each batch's reason afresh, and moves the chosen cards together", async () => {
    const aina: Application = { ...siti, id: "appl_01jbz6w0m0k8q9x2r3s4t5v6w3", candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w3", candidateName: "Aina Yusof" };
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [siti, aina, wei] }));
    const post = vi.spyOn(api, "POST").mockImplementation(((_: string, init: { params: { path: { applicationId: string } } }) =>
      Promise.resolve(ok({ ...siti, id: init.params.path.applicationId, stage: "rejected", version: 4 }))) as never);
    renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate />);
    await screen.findByRole("link", { name: "Aina Yusof" });
    choose("Siti Aisyah");
    moveChosen("rejected");
    fireEvent.change(screen.getByLabelText("Reason"), { target: { value: "not_qualified" } });
    fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "No CPA" } });
    fireEvent.click(screen.getByRole("button", { name: "Rejected 1" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
    choose("Aina Yusof");
    moveChosen("rejected");
    fireEvent.change(screen.getByLabelText("Reason"), { target: { value: "salary_mismatch" } });
    fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "Wants 15k" } });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" })); // changed their mind
    moveChosen("rejected");
    expect(screen.getByLabelText<HTMLSelectElement>("Reason").value).toBe(""); // asked afresh
    expect(screen.getByLabelText<HTMLInputElement>("Note (optional)").value).toBe("");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    choose("Siti Aisyah");
    moveChosen("interview");
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(3); }); // both sent together
    expect(screen.queryByText(/chosen$/)).toBeNull(); // moved ones are unticked
  });

  it("emails the client about one ticked candidate with that candidate's own fields", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(ok({ subject: "", bodyHtml: "", missing: [], salary: false, clientEmail: null, fieldValues: {} }));
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [siti, wei] }));
    renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate permissions={["pipeline:read", "emails:send"]} />);
    await screen.findByRole("link", { name: "Siti Aisyah" });
    choose("Siti Aisyah");
    fireEvent.click(screen.getByRole("button", { name: "Email client about 1 candidate" }));
    const email = await screen.findByRole("region", { name: "Email client" });
    expect(within(email).getByRole("heading", { name: "Email client" })).toBeTruthy(); // about this application, not a list
  });

  it("shows initials, the name and current title, with no tick box; Space on the name chooses the card", async () => {
    await showBoard();
    const name = screen.getByRole("link", { name: "Siti Aisyah" });
    const card = cardOf("Siti Aisyah");
    expect(screen.getByRole("link", { name: "Siti Aisyah" }).getAttribute("title")).toBe("Siti Aisyah · Accountant"); // one line each: the title on hover
    expect(within(card).getByLabelText("Siti Aisyah (status: Not set)").textContent).toBe("SA");
    expect(card.querySelector("input")).toBeNull();
    fireEvent.keyDown(name, { key: " " });
    expect(card.className).toContain("picked");
    expect(card.textContent).toContain("(chosen)");
  });

  it("is read-only without pipeline:update", async () => {
    await showBoard(false);
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(cardOf("Siti Aisyah").getAttribute("draggable")).toBe("false");
  });

  it("emails the client about the candidates ticked, in the board's order", async () => {
    const aina: Application = { ...base, id: "appl_01jbz6w0m0k8q9x2r3s4t5v6w3", candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w3", candidateName: "Aina Yusof", stage: "interview" };
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [aina, siti, wei] }));
    vi.spyOn(api, "POST").mockResolvedValue(ok({ subject: "", bodyHtml: "", missing: [], salary: false, clientEmail: null, fieldValues: {} }));
    renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate permissions={["pipeline:read", "emails:send"]} />);
    await screen.findByRole("link", { name: "Aina Yusof" });
    choose("Aina Yusof");
    choose("Siti Aisyah"); // clicking the card ticks it too
    expect(screen.getByText("2 chosen")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Email client about 2 candidates" }));
    const email = await screen.findByRole("region", { name: "Email client" });
    expect(within(email).getByRole("heading", { name: "Email client about 2 candidates" })).toBeTruthy();
    expect(within(email).getAllByRole("listitem").map((li) => li.querySelector("strong")?.textContent)).toEqual(["Siti Aisyah", "Aina Yusof"]); // screening before interview
  });

  it("offers no ticks to those who may neither email nor move candidates", async () => {
    await showBoard(false);
    expect(screen.queryByRole("checkbox")).toBeNull();
    fireEvent.click(cardOf("Siti Aisyah"));
    expect(screen.queryByText("1 chosen")).toBeNull();
  });

  it("does not tick the card when its name is clicked to open the candidate", async () => {
    await showBoard();
    fireEvent.click(screen.getByRole("link", { name: "Siti Aisyah" })); // the name link
    expect(screen.queryByText("1 chosen")).toBeNull();
  });
});

it("chooses a column with its count, or the whole board with All / None", async () => {
 const applied = { ...siti, id: "appl_applied", candidateId: "cand_applied", candidateName: "Applied Person", stage: "applied" as const };
 vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [siti, applied, wei] }));
 renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate permissions={["pipeline:read", "emails:send"]} />);
 fireEvent.click(await screen.findByRole("button", { name: "Choose all 1 in Applied" }));
 fireEvent.click(screen.getByRole("button", { name: "Choose all 1 in Screening" }));
 expect(screen.getByText("2 chosen")).toBeTruthy();
 fireEvent.click(screen.getByRole("button", { name: "Choose all 1 in Applied" })); // again: unchooses the column only
 expect(cardOf("Siti Aisyah").className).toContain("picked");
 expect(cardOf("Applied Person").className).not.toContain("picked");
 const board = screen.getByRole("group", { name: "Choose" });
 fireEvent.click(within(board).getByRole("button", { name: /^All \d+$/ }));
 expect(within(board).getByRole("button", { name: /^All \d+$/ }).getAttribute("aria-pressed")).toBe("true");
 fireEvent.click(within(board).getByRole("button", { name: "None" }));
 expect(screen.queryByText(/chosen$/)).toBeNull();
});

it("flags who waits too long, the next interview and an offer's deadline", async () => {
  const days = (d: number) => new Date(Date.now() - d * 86_400_000).toISOString();
  const t = new Date(Date.now() + 86_400_000);
  const tomorrow = `${String(t.getFullYear())}-${String(t.getMonth() + 1).padStart(2, "0")}-${String(t.getDate()).padStart(2, "0")}`;
  const stale = { ...siti, stageChangedAt: days(16) };
  const meeting = { ...siti, id: "appl_meet", candidateId: "cand_meet", candidateName: "Mei Lim", stage: "interview" as const, stageChangedAt: days(8) };
  const offered = { ...siti, id: "appl_offer", candidateId: "cand_offer", candidateName: "Aina Liyana", stage: "offer" as const, stageChangedAt: days(2) };
  const at = new Date(Date.now() + 2 * 86_400_000).toISOString();
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/interviews" ? { items: [{ applicationId: "appl_meet", startsAt: at, status: "scheduled" }, { applicationId: "appl_meet", startsAt: at.replace(/^\d{4}/, "2099"), status: "scheduled" }] }
      : path === "/v1/offers" ? { items: [{ applicationId: "appl_offer", expiresOn: tomorrow, status: "sent" }] }
        : { items: [stale, meeting, offered] },
  ))) as never);
  renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate permissions={["pipeline:read", "interviews:read", "offers:read"]} />);
  await screen.findByRole("link", { name: "Siti Aisyah" });
  expect((await within(cardOf("Siti Aisyah")).findByText("16d")).className).toBe("chip red");
  expect(within(cardOf("Mei Lim")).getByText("8d").className).toBe("chip amber");
  expect(await within(cardOf("Aina Liyana")).findByText("answer by tomorrow")).toBeTruthy();
  const chips = cardOf("Mei Lim").querySelectorAll(".chip.cal");
  expect(chips).toHaveLength(1); // the next one only
  expect(chips[0]?.textContent).toBe(new Intl.DateTimeFormat(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }).format(new Date(at)));
});
