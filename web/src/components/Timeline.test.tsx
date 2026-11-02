import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
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

function mockReads(hiddenSensitive = 0) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(path === "/v1/notes" ? ok({ items: [note], hiddenSensitive }) : ok({ items: [change] }))));
}

describe("activity sentences", () => {
  it("say what changed, and never salary amounts", () => {
    expect(sentence(change)).toBe("changed current title from Accountant to Senior Accountant");
    expect(sentence({ ...change, action: "candidate.compensation_updated", changes: { compensation: { changed: true, redacted: true } } }))
      .toBe("changed the salary details");
    expect(sentence({ ...change, action: "contact.updated", changes: { email: { changed: true, redacted: true } } })).toBe("changed email");
    expect(sentence({ ...change, action: "note.created", changes: {} })).toBeNull();
  });
});

describe("timeline", () => {
  it("shows notes and changes together, newest first", async () => {
    mockReads();
    renderWithClient(<Timeline subjectType="candidate" subjectId={subjectId} permissions={["notes:read", "notes.activity:read"]} />);
    const change = await screen.findByText("Aina changed current title from Accountant to Senior Accountant");
    const noteText = screen.getByText("Strong IFRS.");
    expect(change.compareDocumentPosition(noteText) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByRole("form", { name: "Add a note" })).toBeNull();
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
    renderWithClient(<Dashboard />);
    await screen.findByRole("link", { name: "Auditor" });
    expect(screen.getByText("candidates in progress").previousSibling?.textContent).toBe("4");
    expect(screen.getByText("moved this week").previousSibling?.textContent).toBe("3");
    expect(screen.getByText(/Nobody has waited more than 14 days/)).toBeTruthy();
  });
});
