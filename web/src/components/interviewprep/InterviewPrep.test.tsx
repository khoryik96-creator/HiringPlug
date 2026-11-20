import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import { MODULE } from "../../labels";
import { JobDetail } from "../../pages/JobDetail";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../../test-utils";
import { InterviewPrepPanel, prepText } from "./InterviewPrepPanel";
import { InterviewQuestions } from "./InterviewQuestions";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const question = (over: Partial<Schemas["InterviewQuestion"]> = {}): Schemas["InterviewQuestion"] => ({
  id: "ivq_1", jobId: "jo_1", question: "What if the nightly batch is late on month end?", round: "hiring_manager", kind: "situational",
  askedOn: "2026-11-18", source: "Candidate after round 2", notes: "They wanted a stakeholder plan", archived: false, version: 1,
  createdAt: "2026-11-18T03:00:00Z", updatedAt: "2026-11-18T03:00:00Z", ...over,
});

const item = (over: Partial<Schemas["InterviewPrepItem"]> = {}): Schemas["InterviewPrepItem"] => ({
  question: "What if the nightly batch is late on month end?", kind: "What-if", whyAsked: "Tests calm prioritisation under a deadline.",
  approach: ["Say who is affected first", "Contain, then fix"], phrasing: ["My first call would be to [stakeholder] to agree what can wait."],
  tradeOffs: ["Speed vs completeness: speed when regulators wait"], pitfalls: ["Jumping to the technical fix"], fromInterviews: true, ...over,
});

const content: Schemas["InterviewPrepContent"] = {
  summary: "Expect depth on streaming reliability and calm incident handling.", focusAreas: ["Kafka delivery guarantees", "Month-end reporting"],
  softSkills: [item()],
  technical: [item({ question: "How do you make a Spark job idempotent?", kind: "Technical", fromInterviews: false,
    phrasing: ["I'd key each write on [business key] so a rerun overwrites, never doubles."] })],
  questionsToAsk: ["How is data quality owned today?"],
};

const prep = (over: Partial<Schemas["InterviewPrep"]> = {}): Schemas["InterviewPrep"] => ({
  id: "ivp_1", jobId: "jo_1", status: "done", focus: "New to banking", content, error: null, model: "test-model", questionCount: 1,
  archived: false, createdAt: "2026-11-19T02:00:00Z", finishedAt: "2026-11-19T02:01:00Z", ...over,
});

describe("questions asked at a job's interviews", () => {
  it("records a question with its round and who told it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(question()));
    renderWithClient(<InterviewQuestions jobId="jo_1" canManage />);
    expect(await screen.findByText(/No questions recorded yet/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Record a question" }));
    const form = screen.getByRole("form", { name: "Record a question" });
    fireEvent.change(within(form).getByLabelText("Question"), { target: { value: "What if the nightly batch is late on month end?" } });
    fireEvent.change(within(form).getByLabelText("Kind"), { target: { value: "situational" } });
    fireEvent.change(within(form).getByLabelText("Round"), { target: { value: "hiring_manager" } });
    fireEvent.change(within(form).getByLabelText("Told by"), { target: { value: "  Candidate after round 2 " } });
    fireEvent.click(within(form).getByRole("button", { name: "Save question" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ question: "What if the nightly batch is late on month end?", kind: "situational", round: "hiring_manager",
      askedOn: null, source: "Candidate after round 2", notes: null });
  });

  it("archives a question and brings it back from the archive", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [question()] }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(question({ archived: true, version: 2 })));
    renderWithClient(<InterviewQuestions jobId="jo_1" canManage />);
    expect(await screen.findByText("Situational / what-if")).toBeTruthy();
    expect(screen.getByText("Hiring manager round")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: 'Archive "What if the nightly batch is late on month end?"' }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 1, archived: true });

    get.mockResolvedValue(ok({ items: [question({ archived: true, version: 2 })] }));
    fireEvent.click(screen.getByRole("button", { name: "Archived" }));
    await waitFor(() => { expect(get).toHaveBeenLastCalledWith("/v1/jobs/{jobId}/interview-questions", { params: { path: { jobId: "jo_1" }, query: { archived: true } } }); });
    fireEvent.click(await screen.findByRole("button", { name: 'Bring back "What if the nightly batch is late on month end?"' }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
    expect(sentBody(patch, 1)).toEqual({ version: 2, archived: false });
  });

  it("closes the record form when switching to the archived questions", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<InterviewQuestions jobId="jo_1" canManage />);
    fireEvent.click(await screen.findByRole("button", { name: "Record a question" }));
    expect(screen.getByRole("form", { name: "Record a question" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Archived" }));
    expect(screen.queryByRole("form", { name: "Record a question" }), "a question saved there would vanish from view").toBeNull();
  });

  it("lets a sourcer without manage rights only read", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [question()] }));
    renderWithClient(<InterviewQuestions jobId="jo_1" canManage={false} />);
    expect(await screen.findByText("What if the nightly batch is late on month end?")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Record a question" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Archive "/ })).toBeNull();
  });
});

describe("interview prep written by the AI", () => {
  it("asks for prep with a focus, and follows it while it is written", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [prep({ status: "queued", content: null, finishedAt: null })] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(prep({ status: "queued", content: null })));
    renderWithClient(<InterviewPrepPanel jobId="jo_1" jobTitle="Data Engineer" canManage />);
    expect((await screen.findByRole("status")).textContent).toContain("The AI is writing it");
    fireEvent.change(screen.getByLabelText("Focus (optional)"), { target: { value: " New to banking " } });
    fireEvent.click(screen.getByRole("button", { name: "Write prep with AI" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ focus: "New to banking" });
  });

  it("shows each question as a short card: why, steps, example lines, trade-offs and what to avoid", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [prep()] }));
    renderWithClient(<InterviewPrepPanel jobId="jo_1" jobTitle="Data Engineer" canManage />);
    expect(await screen.findByText("Expect depth on streaming reliability and calm incident handling.")).toBeTruthy();
    expect(screen.getByText("Kafka delivery guarantees")).toBeTruthy();
    const asked = screen.getByText("What if the nightly batch is late on month end?").closest("details") as HTMLDetailsElement;
    expect(asked.open, "the first of each section is open").toBe(true);
    expect(within(asked).getByText("Asked at interview")).toBeTruthy();
    expect(within(asked).getByText("Tests calm prioritisation under a deadline.")).toBeTruthy();
    expect(within(asked).getAllByRole("listitem").map((li) => li.textContent)).toContain("Contain, then fix");
    expect(asked.querySelector("blockquote")?.textContent).toBe("My first call would be to [stakeholder] to agree what can wait.");
    expect(within(asked).getByText("Avoid")).toBeTruthy();
    const technical = screen.getByText("How do you make a Spark job idempotent?").closest("details") as HTMLDetailsElement;
    expect(within(technical).queryByText("Asked at interview"), "the AI's own questions are not marked").toBeNull();
    expect(screen.getByText("How is data quality owned today?")).toBeTruthy();
  });

  it("says why the AI could not write it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [prep({ status: "failed", content: null, error: "the AI provider is busy" })] }));
    renderWithClient(<InterviewPrepPanel jobId="jo_1" jobTitle="Data Engineer" canManage />);
    expect((await screen.findByText(/the AI provider is busy/)).textContent).toContain("Try again");
  });

  it("keeps the last good prep open when a newer one failed", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      prep({ id: "ivp_2", status: "failed", content: null, error: "the AI provider is busy", createdAt: "2026-11-20T02:00:00Z" }),
      prep(),
    ] }));
    renderWithClient(<InterviewPrepPanel jobId="jo_1" jobTitle="Data Engineer" canManage />);
    expect((await screen.findByText(/the AI provider is busy/)).textContent).toBe("the AI provider is busy. Try again.");
    const summary = screen.getByText("Expect depth on streaming reliability and calm incident handling.");
    expect((summary.closest("details.prep-card") as HTMLDetailsElement).open).toBe(true);
    expect(summary.closest(".prep-earlier"), "not folded away under Earlier prep").toBeNull();
  });

  it("shows a line the AI repeated twice, without duplicate keys", async () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const twice = { ...content, softSkills: [item({ pitfalls: ["Jumping to the fix", "Jumping to the fix"] })], focusAreas: ["Kafka", "Kafka"] };
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [prep({ content: twice })] }));
    renderWithClient(<InterviewPrepPanel jobId="jo_1" jobTitle="Data Engineer" canManage />);
    expect(await screen.findAllByText("Jumping to the fix")).toHaveLength(2);
    expect(errors.mock.calls.flat().join(" ")).not.toMatch(/same key/);
  });

  it("copies over plain http too, and says so for a moment", async () => {
    const exec = vi.fn(() => true);
    Object.defineProperty(document, "execCommand", { value: exec, configurable: true });
    expect((navigator as { clipboard?: unknown }).clipboard, "no Clipboard API, as on http").toBeUndefined();
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [prep()] }));
    renderWithClient(<InterviewPrepPanel jobId="jo_1" jobTitle="Data Engineer" canManage />);
    fireEvent.click(await screen.findByRole("button", { name: "Copy as text" }));
    expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
    expect(exec).toHaveBeenCalledWith("copy");
    expect(await screen.findByRole("button", { name: "Copy as text" }, { timeout: 3000 }), "back after a moment").toBeTruthy();
  });

  it("archives a prep", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [prep()] }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(prep({ archived: true })));
    renderWithClient(<InterviewPrepPanel jobId="jo_1" jobTitle="Data Engineer" canManage />);
    fireEvent.click(await screen.findByRole("button", { name: "Archive this prep" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ archived: true });
  });

  it("copies as plain text, easy to paste to a candidate", () => {
    const text = prepText("Data Engineer", content);
    expect(text).toContain("Interview prep: Data Engineer");
    expect(text).toContain("SOFT SKILLS\n1. What if the nightly batch is late on month end? (asked at interview)\n  Why they ask: Tests calm prioritisation under a deadline.");
    expect(text).toContain("  Say it like:\n    - My first call would be to [stakeholder] to agree what can wait.");
    expect(text).toContain("TECHNICAL\n1. How do you make a Spark job idempotent?\n");
    expect(text).toContain("Questions to ask them:\n  - How is data quality owned today?");
  });
});

describe("the job page's interview insights tab", () => {
  it("is named in Roles and access", () => {
    expect(MODULE.interview_prep).toBe("Interview insights");
  });

  const job = { id: "jo_1", title: "Data Engineer", companyId: "comp_1", companyName: "Maju Bank Berhad", status: "open", location: "Kuala Lumpur",
    country: "MY", openings: 1, ownerId: null, ownerName: null, description: null, intakeDate: null, createdAt: "2026-11-01T00:00:00Z",
    updatedAt: "2026-11-01T00:00:00Z" };
  const mockApi = () => vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/jobs/{jobId}" ? ok(job) : path === "/v1/settings/fields" ? testFieldSettings() : ok({ items: [] }))) as never);

  it("shows only to those who may read interview insights", async () => {
    mockApi();
    renderWithClient(<JobDetail id="jo_1" permissions={["jobs:read", "interview_prep:read"]} />);
    fireEvent.click(await screen.findByRole("tab", { name: "Interview insights" }));
    expect(await screen.findByRole("region", { name: "Questions asked" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Interview prep" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Write prep with AI" }), "reading is not managing").toBeNull();
    cleanup();
    mockApi();
    renderWithClient(<JobDetail id="jo_1" permissions={["jobs:read"]} />);
    expect(await screen.findByRole("tab", { name: "Job details" })).toBeTruthy();
    expect(screen.queryByRole("tab", { name: "Interview insights" })).toBeNull();
  });
});
