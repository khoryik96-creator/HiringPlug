import { cleanup, fireEvent, screen, within, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type ActivityEntry, type Note } from "../api/client";
import { Dashboard } from "../pages/Dashboard";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { describe as sentence } from "./activityText";
import { SCREENING_TEMPLATE, Timeline } from "./Timeline";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const subjectId = "cand_01jbz6w0m0k8q9x2r3s4t5v6w7";
const note: Note = {
  id: "note_01jbz6w0m0k8q9x2r3s4t5v6w7", subjectType: "candidate", subjectId, kind: "screening", body: "Strong IFRS.",
  sensitive: false, authorId: "usr_01jbz6w0m0k8q9x2r3s4t5v6w7", authorName: "Aina", createdAt: "2026-11-02T09:00:00Z",
};
const change: ActivityEntry = {
  id: "aud_1", action: "candidate.updated", actorName: "Aina", at: "2026-11-02T10:00:00Z",
  changes: { currentTitle: { from: "Accountant", to: "Senior Accountant" } },
};

function mockReads(hiddenSensitive = 0, notes: Note[] = [note], calls: unknown[] = []) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/notes" ? ok({ items: notes, hiddenSensitive })
      : path === "/v1/candidates/{candidateId}/screening-calls" ? ok({ items: calls })
        : ok({ items: [change] }))) as never);
}

describe("activity sentences", () => {
  it("say what changed, and never salary amounts", () => {
    expect(sentence(change)).toBe("changed current title from Accountant to Senior Accountant");
    expect(sentence({ ...change, action: "candidate.compensation_updated", changes: { compensation: { changed: true, redacted: true } } }))
      .toBe("changed the salary details");
    expect(sentence({ ...change, action: "contact.updated", changes: { email: { changed: true, redacted: true } } })).toBe("changed email");
    expect(sentence({ ...change, action: "note.created", changes: {} })).toBeNull();
  });

  it("never break on entries of another shape (a merge's empty list broke candidate pages)", () => {
    expect(sentence({ ...change, action: "candidate.merged", changes: { filled: null, duplicateId: "cand_2", duplicateName: "Mohd Faizal" } }))
      .toBe("merged in a duplicate record (Mohd Faizal)");
    expect(sentence({ ...change, action: "candidate.merged", changes: { filled: ["email", "currentTitle"], duplicateName: "A" } }))
      .toBe("merged in a duplicate record (A), taking email, current title from it");
    expect(sentence({ ...change, action: "candidate.filled_from_cv", changes: { email: { from: null, to: "a@b.c" }, phone: { from: null, to: "1" } } }))
      .toBe("filled in from their CV: email, phone");
    expect(sentence({ ...change, action: "thing.done", changes: { a: null, b: "x", c: ["p", "q"], d: { odd: 1 } } }))
      .toBe("a: —; b: x; c: p, q; changed d");
  });

  it("tell what an employment check did", () => {
    expect(sentence({ ...change, action: "contact.employment_check_started", changes: { fileName: "Profile.pdf" } }))
      .toBe("checked their LinkedIn profile (Profile.pdf)");
    expect(sentence({ ...change, action: "contact.employment_checked", changes: { status: "read", linkedinUrl: { from: null, to: "x" } } }))
      .toBe("read their LinkedIn profile and added their LinkedIn address");
    expect(sentence({ ...change, action: "contact.employment_checked", changes: { status: "failed" } }))
      .toBe("could not read their LinkedIn profile");
  });
});

describe("timeline", () => {
  it("shows notes and changes together, newest first", async () => {
    mockReads(0, [{ ...note, kind: "general" }]);
    renderWithClient(<Timeline subjectType="candidate" subjectId={subjectId} permissions={["notes:read", "notes.activity:read"]} />);
    const change = await screen.findByText("Aina changed current title from Accountant to Senior Accountant");
    const noteText = screen.getByText("Strong IFRS.");
    expect(change.compareDocumentPosition(noteText) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByRole("form", { name: "Add a note" })).toBeNull();
  });

  it("pins the latest call, filters by kind, searches, and folds long notes", async () => {
    const older = { ...note, id: "note_old", body: "RFL: restructuring\nLF: stable MNC", createdAt: "2026-10-21T09:00:00Z", authorName: "Hafiz" };
    const latest = { ...note, id: "note_new", body: "Call on 8 Nov 2026 by Aina\n\nRFL: wants regional scope\nLF: logistics, hybrid", createdAt: "2026-11-09T09:00:00Z", authorName: "Hafiz" };
    const long = { ...note, id: "note_long", kind: "general" as const, body: `Client feedback: ${"strong on hub operations. ".repeat(12)}Wants a second round.` };
    mockReads(0, [latest, long, older], [
      { id: "scl_2", calledOn: "2026-11-08", calledBy: "usr_1", calledByName: "Aina", noteId: "note_new", salaryRecorded: true, createdAt: "2026-11-09T09:00:00Z" },
      { id: "scl_1", calledOn: "2026-10-21", calledBy: "usr_2", calledByName: "Hafiz", noteId: "note_old", salaryRecorded: false, createdAt: "2026-10-21T09:00:00Z" },
    ]);
    renderWithClient(<Timeline subjectType="candidate" subjectId={subjectId} permissions={["notes:read", "notes.activity:read", "candidates:read"]} />);
    // The latest screening call is the open card on top; earlier ones are folded under it.
    const pinned = await screen.findByLabelText("Latest call");
    expect(pinned.textContent).toContain("8 Nov 2026 · Aina"); // when it was, not when it was logged
    expect(pinned.textContent).toContain("Salary updated");
    expect(pinned.textContent).toContain("RFLwants regional scope");
    expect(pinned.textContent).not.toContain("Call on 8 Nov 2026"); // in the header already
    expect(screen.queryAllByText(/wants regional scope/)).toHaveLength(1); // not twice
    const list = screen.getByRole("list", { name: "Notes and changes" });

    const folded = within(list).getByText(/Client feedback/);
    expect(folded.className).toContain("folded");
    fireEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(folded.className).not.toContain("folded");

    fireEvent.click(screen.getByRole("button", { name: /^Calls/ }));
    expect(within(list).queryByText(/Client feedback/)).toBeNull();
    expect(within(list).getByText(/restructuring/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /^All/ }));
    fireEvent.change(screen.getByLabelText("Search notes"), { target: { value: "second round" } });
    expect(within(list).getByText(/Client feedback/)).toBeTruthy();
    expect(within(list).queryByText(/restructuring/)).toBeNull();
    expect(screen.queryByText(/changed current title/)).toBeNull();
  });

  it("says how many sensitive notes are hidden", async () => {
    mockReads(2);
    renderWithClient(<Timeline subjectType="candidate" subjectId={subjectId} permissions={["notes:read"]} />);
    expect(await screen.findByText(/2 sensitive notes are hidden for your role/)).toBeTruthy();
  });

  it("starts a screening note with the questions, and saves it", async () => {
    mockReads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(note));
    renderWithClient(<Timeline subjectType="candidate" subjectId={subjectId} permissions={["notes:read", "notes:create", "notes.sensitive:read"]} />);
    fireEvent.change(screen.getByLabelText("Kind"), { target: { value: "screening" } });
    expect(screen.getByLabelText<HTMLTextAreaElement>("Note").value).toBe(SCREENING_TEMPLATE);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Add note" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ subjectType: "candidate", subjectId, kind: "screening", body: SCREENING_TEMPLATE, sensitive: true });
  });
});

describe("dashboard", () => {
  it("shows the week's numbers, what needs you today, and the open jobs as a heat map", async () => {
    const counts = { applied: 2, screening: 1, submitted: 3, interview: 1, offer: 0, placed: 1 };
    const none = { applied: 0, screening: 0, submitted: 0, interview: 0, offer: 0, placed: 0 };
    const jobId = "jo_01jbz6w0m0k8q9x2r3s4t5v6w7";
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/pipeline/jobs"
      ? { items: [{ jobId, stageCounts: counts, lastMovedAt: new Date(Date.now() - 9 * 86_400_000).toISOString(), waiting: 2, waitingCounts: { ...none, submitted: 2 } }] }
      : {
        openJobs: 1, stageCounts: counts, placedLast30Days: 1, movedLast7Days: 3, movedPrev7Days: 8, addedLast7Days: 5, openedLast7Days: 1,
        jobs: [{ jobId, title: "Auditor", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank", stageCounts: counts }],
        stuck: [],
      }))) as never);
    renderWithClient(<Dashboard permissions={["pipeline:read"]} />);
    const job = (await screen.findByRole("link", { name: "Auditor" })).closest("tr") as HTMLElement;
    const kpi = (label: string) => (screen.getByText(label, { selector: ".kpi-label" }).closest(".kpi") as HTMLElement).textContent;
    expect(kpi("In progress")).toBe("In progress7▲ 5 added this week");
    expect(kpi("Moved this week")).toBe("Moved this week3▼ 5 vs last week");
    expect(kpi("Open jobs")).toBe("Open jobs1▲ 1 new this week");
    await waitFor(() => { expect(job.querySelector(".last.old")?.textContent).toMatch(/^\d+ days ago$/); });
    expect(within(job).getByText("3").className).toBe("heat h2 stall"); // 3 submitted, 2 of them waiting
    expect(within(job).getByText("2").className).toBe("heat h2");
    const needs = screen.getByRole("region", { name: "Needs you today" });
    expect(needs.textContent).toContain("2 waiting over 7 days · Auditor, Maybank · Submitted 2");
    expect(within(needs).getByRole("link", { name: /2 waiting/ }).getAttribute("href")).toBe(`#/jobs/${jobId}`);
  });

});

it("shows protected call history even when the call had no general note", async()=>{
 mockReads(0,[],[{id:"scl_salary",calledOn:"2026-11-08",calledBy:"usr_1",calledByName:"Aina",noteId:null,salaryRecorded:true,createdAt:"2026-11-08T10:00:00Z",salarySnapshot:{currentSalary:null,expectedSalary:null,salaryNotes:"Recorded salary discussion"}}]);
 const view=renderWithClient(<Timeline subjectType="candidate" subjectId={subjectId} permissions={["candidates:read","candidates.compensation:read"]}/>);
 // The call's card is on top of Notes and activity, open, with the salary given on it.
 expect(await screen.findByText("Recorded salary discussion")).toBeTruthy();
 view.rerender(<Timeline subjectType="candidate" subjectId={subjectId} permissions={["candidates:read"]}/>);
 expect(screen.getByLabelText("Latest call")).toBeTruthy();expect(screen.queryByText("Recorded salary discussion")).toBeNull();
});

it("shows the five most urgent things needing you, then all on request", async () => {
  const none = { applied: 0, screening: 0, submitted: 0, interview: 0, offer: 0, placed: 0 };
  const jobs = Array.from({ length: 7 }, (_, i) => ({ jobId: `jo_${String(i)}`, title: `Job ${String(i)}`, companyId: "comp_1", companyName: "Maybank", stageCounts: { ...none, screening: i + 1 } }));
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/pipeline/jobs"
    ? { items: jobs.map((j, i) => ({ jobId: j.jobId, stageCounts: j.stageCounts, lastMovedAt: null, waiting: i + 1, waitingCounts: { ...none, screening: i + 1 } })) }
    : { openJobs: 7, stageCounts: none, placedLast30Days: 0, movedLast7Days: 0, movedPrev7Days: 0, addedLast7Days: 0, openedLast7Days: 0, jobs, stuck: [] }))) as never);
  renderWithClient(<Dashboard permissions={["pipeline:read"]} />);
  const needs = await screen.findByRole("region", { name: "Needs you today" });
  await waitFor(() => { expect(needs.querySelectorAll(".todo")).toHaveLength(5); });
  expect(needs.querySelector(".todo")?.textContent).toContain("7 waiting over 7 days · Job 6"); // most people waiting first
  fireEvent.click(within(needs).getByRole("button", { name: "Show all 7" }));
  expect(needs.querySelectorAll(".todo")).toHaveLength(7);
});
