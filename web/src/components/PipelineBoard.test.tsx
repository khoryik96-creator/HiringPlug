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

/** Clicking a card (not its name) ticks it. */
/** A candidate's card on the board, by their name. */
const cardOf = (name: string) => { const card = screen.getByRole("link", { name }).closest(".card"); if (!card) throw new Error("no card"); return card; };
const choose = (name: string) => { fireEvent.click(cardOf(name)); };
const moveChosen = (stage: string) => { fireEvent.change(screen.getByRole("combobox", { name: "Move the chosen candidates" }), { target: { value: stage } }); };

async function showBoard(canUpdate = true) {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [siti, wei] }));
  renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate={canUpdate} />);
  await screen.findByRole("link", { name: "Siti Aisyah" });
}

describe("pipeline board", () => {
  it("shows each candidate in their stage, and closed ones with the reason", async () => {
    await showBoard();
    expect(within(screen.getByRole("list", { name: "Screening" })).getByRole("link", { name: "Siti Aisyah" })).toBeTruthy();
    expect(within(screen.getByRole("list", { name: "Screening" })).getByText("3 days in screening")).toBeTruthy();
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

  it("shows the name with no tick box and no title; Space on the name chooses the card", async () => {
    await showBoard();
    const name = screen.getByRole("link", { name: "Siti Aisyah" });
    const card = cardOf("Siti Aisyah");
    expect(card.textContent).not.toContain("Accountant");
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

it("selects and clears each early stage without losing candidates in other stages",async()=>{
 const applied={...siti,id:"appl_applied",candidateId:"cand_applied",candidateName:"Applied Person",stage:"applied"};
 vi.spyOn(api,"GET").mockResolvedValue(ok({items:[siti,applied,wei]}));
 renderWithClient(<PipelineBoard jobId={base.jobId} canUpdate permissions={["pipeline:read","emails:send"]}/>);
 fireEvent.click(await screen.findByRole("button",{name:"Select all Applied candidates"}));
 fireEvent.click(screen.getByRole("button",{name:"Select all Screening candidates"}));
 fireEvent.click(screen.getByRole("button",{name:"Select all Screening candidates"}));
 expect(screen.getByText("2 chosen")).toBeTruthy();
 fireEvent.click(screen.getByRole("button",{name:"Unselect all Applied candidates"}));
 expect(cardOf("Siti Aisyah").className).toContain("picked");
 expect(cardOf("Applied Person").className).not.toContain("picked");
 fireEvent.click(screen.getByRole("button",{name:"Unselect all Screening candidates"}));
 expect(screen.queryByText("1 chosen")).toBeNull();
});
