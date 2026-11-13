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
    expect(sentence({ ...change, action: "consent.kept_withdrawn", changes: { method: "careers_form" } }))
      .toContain("their consent stays withdrawn until someone confirms it with them");
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
  it("shows open jobs, totals and who is waiting too long", async () => {
    const counts = { applied: 2, screening: 1, submitted: 0, interview: 1, offer: 0, placed: 1 };
    vi.spyOn(api, "GET").mockResolvedValue(ok({
      openJobs: 1, stageCounts: counts, placedLast30Days: 1, movedLast7Days: 3,
      jobs: [{ jobId: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", title: "Auditor", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank", stageCounts: counts }],
      stuck: [],
    }));
    renderWithClient(<Dashboard permissions={["pipeline:read"]} />);
    await screen.findByRole("link", { name: "Auditor" });
    expect(screen.getByText("candidates in progress").previousSibling?.textContent).toBe("4");
    expect(screen.getByText("moved this week").previousSibling?.textContent).toBe("3");
    expect(screen.getByText(/Nobody has waited more than 14 days/)).toBeTruthy();
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
