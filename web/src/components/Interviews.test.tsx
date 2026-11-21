import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { failure, ok, RECRUITER, renderWithClient, sentBody, testMe } from "../test-utils";
import { ApplicationInterviews, InterviewItem } from "./Interviews";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const me = testMe(RECRUITER); // user usr_01jbz6w0m0k8q9x2r3s4t5v6w7
const interview = (over: Partial<Schemas["Interview"]> = {}): Schemas["Interview"] => ({
  id: "intv_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah",
  jobId: "jo_1", jobTitle: "Accountant", title: "First interview", startsAt: "2026-11-10T02:00:00Z", durationMinutes: 60,
  mode: "video", location: "https://meet.example/abc", instructions: null, status: "scheduled",
  interviewers: [{ userId: me.user.id, name: "Demo" }, { userId: "usr_2", name: "Aina" }],
  feedback: [], feedbackHidden: 0, myFeedbackDue: false, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  ...over,
});
const reads = (items: Schemas["Interview"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/me") return Promise.resolve(ok(me));
    if (path === "/v1/users") return Promise.resolve(ok({ items: [{ id: me.user.id, name: "Demo", role: "recruiter", canInterview: true }, { id: "usr_2", name: "Aina", role: "hiring_manager", canInterview: true }, { id: "usr_3", name: "Vee", role: "viewer", canInterview: false }] }));
    return Promise.resolve(ok({ items }));
  }) as never);

/** The composer's date and start time, as a user picks them. */
const pickTime = () => {
  fireEvent.change(screen.getByLabelText("Date"), { target: { value: "2026-11-10" } });
  fireEvent.change(screen.getByLabelText("Start time"), { target: { value: "10:00" } });
};
const addInterviewer = async (id: string) => {
  const add = await screen.findByRole("combobox", { name: "Add interviewer" });
  await within(add).findByRole("option", { name: "Aina" });
  fireEvent.change(add, { target: { value: id } });
};
const connected = { connected: true, ready: true, status: "ok", callbackUrl: "", version: 1, address: "me@agency.test", calendarName: null, lastSyncAt: null, error: null, teamsAvailable: true };
const CALENDAR: Schemas["Permission"][] = [...RECRUITER, "interviews:read", "interviews:schedule", "calendars:use"];
/** Reads for a user whose Outlook is connected (Teams available unless said otherwise). */
const readsWithOutlook = (teamsAvailable = true) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/me") return Promise.resolve(ok(me));
    if (path === "/v1/calendar/connection") return Promise.resolve(ok({ ...connected, teamsAvailable }));
    if (path === "/v1/calendar/events") return Promise.resolve(ok({ items: [], connection: connected }));
    if (path === "/v1/email-signature") return Promise.resolve(ok({ bodyHtml: "<p>Best, Demo</p>", version: 1 }));
    if (path === "/v1/email-templates") return Promise.resolve(ok({ fields: [], placeholders: [], items: [
      { id: "etp_1", version: 1, name: "Interview invitation", subject: "Interview for {{job.title}}", body: "Hi", bodyHtml: null, createdAt: "", updatedAt: "" },
      { id: "etp_2", version: 1, name: "Offer chat", subject: "Your offer", body: "We can offer {{candidate.expectedSalary}}", bodyHtml: null, createdAt: "", updatedAt: "" },
    ] }));
    if (path === "/v1/users") return Promise.resolve(ok({ items: [{ id: me.user.id, name: "Demo", role: "recruiter", canInterview: true }, { id: "usr_2", name: "Aina", role: "hiring_manager", canInterview: true }] }));
    return Promise.resolve(ok({ items: [] }));
  }) as never);

describe("interviews", () => {
  it("schedules with the chosen interviewers, and offers to go ahead when someone is busy", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(failure("INTERVIEW_CLASH", "An interviewer already has an interview at that time.", {
        clashes: [{ interviewId: "intv_x", title: "Panel", startsAt: "2026-11-10T02:30:00Z", interviewers: ["Aina"] }],
      }, 409))
      .mockResolvedValueOnce(ok(interview()));
    renderWithClient(<ApplicationInterviews applicationId="appl_1" closed={false} permissions={[...RECRUITER, "interviews:read", "interviews:schedule"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    const add = await screen.findByRole("combobox", { name: "Add interviewer" });
    await within(add).findByRole("option", { name: "Aina" });
    expect(within(add).queryByRole("option", { name: "Vee" })).toBeNull(); // viewers cannot interview
    fireEvent.change(add, { target: { value: "usr_2" } });
    fireEvent.click(screen.getByRole("button", { name: "Schedule" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Aina: Panel");
    expect(sentBody(post)).toMatchObject({ title: "First interview", mode: "video", interviewerIds: ["usr_2"], ignoreClashes: false });
    fireEvent.click(within(alert).getByRole("button", { name: "Schedule anyway" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post, 1)).toMatchObject({ ignoreClashes: true });
  });

  it("offers to go ahead over a busy or unsynced calendar too, naming who", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(failure("INTERVIEW_CLASH", "An interviewer has a calendar appointment at that time.", {
        clashes: [], calendarBusy: ["Aina"], calendarStale: ["Demo"],
      }, 409))
      .mockResolvedValueOnce(ok(interview()));
    renderWithClient(<ApplicationInterviews applicationId="appl_1" closed={false} permissions={[...RECRUITER, "interviews:read", "interviews:schedule"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    await addInterviewer("usr_2");
    fireEvent.click(screen.getByRole("button", { name: "Schedule" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Aina: an appointment in their calendar");
    expect(alert.textContent).toContain("Demo: their Outlook has not synced lately");
    fireEvent.click(within(alert).getByRole("button", { name: "Schedule anyway" })); // there was none without named clashes
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post, 1)).toMatchObject({ ignoreClashes: true });
  });

  it("schedules a Zoom interview and sends the Outlook invitation in the same step", async () => {
    readsWithOutlook();
    const post = vi.spyOn(api, "POST").mockResolvedValueOnce(ok(interview({ version: 1 }))).mockResolvedValueOnce(ok({ status: "pending" }));
    renderWithClient(<ApplicationInterviews applicationId="appl_1" candidateName="Siti Aisyah" closed={false} permissions={CALENDAR} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    await addInterviewer("usr_2");
    expect(screen.getByText("Siti Aisyah")).toBeTruthy(); // the candidate is invited
    fireEvent.click(screen.getByRole("radio", { name: "Zoom" }));
    const link = screen.getByLabelText("Zoom link");
    fireEvent.change(link, { target: { value: "https://meet.google.com/abc-defg-hij" } });
    fireEvent.blur(link);
    expect(screen.getByRole("alert").textContent).toBe("This is not a Zoom link.");
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => { expect(screen.getAllByText("This is not a Zoom link.")).toHaveLength(2); }); // refused on sending too
    expect(post).not.toHaveBeenCalled();
    fireEvent.change(link, { target: { value: "https://us02web.zoom.us/j/8123456789?pwd=abc" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post)).toMatchObject({ mode: "video", location: "https://us02web.zoom.us/j/8123456789?pwd=abc", interviewerIds: ["usr_2"] });
    expect(post.mock.calls[1]?.[0]).toBe("/v1/interviews/{interviewId}/outlook-invitation");
    expect(sentBody(post, 1)).toMatchObject({ version: 1, teams: false, reminderMinutes: 15 });
  });

  it("writes the invitation from a template, with the signature and optional attendees", async () => {
    readsWithOutlook();
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(path === "/v1/email-drafts"
      ? ok({ to: [], subject: "Interview for Accountant on [Interview time]", body: "", missing: [],
        bodyHtml: "<p>Dear Siti, we meet on {{interview.when}} ({{interview.length}}) at [Interview place].</p>" })
      : path === "/v1/applications/{applicationId}/interviews" ? ok(interview({ version: 3 })) : ok({ status: "pending" }))) as never);
    renderWithClient(<ApplicationInterviews applicationId="appl_1" candidateId="cand_1" closed={false} permissions={[...CALENDAR, "emails:send"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    await addInterviewer("usr_2");
    await waitFor(() => { expect(screen.getByRole("button", { name: "Signature" }).getAttribute("aria-pressed")).toBe("true"); }); // added, as Outlook does
    fireEvent.click(screen.getByRole("radio", { name: "Google Meet" }));
    fireEvent.change(screen.getByLabelText("Google Meet link"), { target: { value: "https://meet.google.com/abc-defg-hij" } });
    fireEvent.click(screen.getByRole("button", { name: "Template" }));
    expect(await screen.findByRole("menuitem", { name: "Interview invitation" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Offer chat" })).toBeNull(); // salary never goes into an invitation
    fireEvent.click(screen.getByRole("menuitem", { name: "Interview invitation" }));
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Title").value).toBe("Interview for Accountant"); }); // no time: it would go stale in the title
    const cc = screen.getByLabelText("Optional attendees");
    fireEvent.change(cc, { target: { value: "aina@client.test" } }); fireEvent.keyDown(cc, { key: "Enter" });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    const calls = post.mock.calls as unknown as [string, { body: Record<string, unknown> }][];
    await waitFor(() => { expect(calls.some(([p]) => p === "/v1/interviews/{interviewId}/outlook-invitation")).toBe(true); });
    const created = calls.find(([p]) => p === "/v1/applications/{applicationId}/interviews")?.[1];
    expect(created?.body.instructions).toBeNull(); // a template's message is the invitation, not the instructions (they would keep its time)
    const sent = calls.find(([p]) => p === "/v1/interviews/{interviewId}/outlook-invitation")?.[1];
    expect(sent?.body.optional).toEqual(["aina@client.test"]);
    expect(sent?.body.bodyHtml).toContain("Dear Siti");
    const when = new Date(2026, 10, 10, 10).toLocaleString(undefined, { weekday: "long", day: "numeric", month: "long", hour: "numeric", minute: "2-digit" });
    expect(sent?.body.bodyHtml).toContain(`<span data-field="interview.when">${when}</span>`); // marked, so it follows the interview
    expect(sent?.body.bodyHtml).toContain('<span data-field="interview.where">https://meet.google.com/abc-defg-hij</span>');
    expect(sent?.body.bodyHtml).toContain('data-signature="true"');
  });

  it("lets Outlook make the Teams meeting, and keeps the interview when the invitation fails", async () => {
    readsWithOutlook();
    const post = vi.spyOn(api, "POST").mockResolvedValueOnce(ok(interview({ version: 2 })))
      .mockResolvedValueOnce(failure("INTERVIEW_NOT_ALLOWED", "Add a valid email address to the candidate profile before sending.", undefined, 422));
    renderWithClient(<ApplicationInterviews applicationId="appl_1" closed={false} permissions={CALENDAR} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    await addInterviewer("usr_2");
    expect((await screen.findByRole("radio", { name: "Teams" })).getAttribute("aria-checked")).toBe("true"); // Teams by default when Outlook offers it
    expect(screen.getByText(/Outlook adds the Teams link/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Reminder"), { target: { value: "60" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("The interview is scheduled.")).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toContain("Add a valid email address");
    expect(sentBody(post)).toMatchObject({ mode: "video", location: "Microsoft Teams Meeting" });
    expect(sentBody(post, 1)).toMatchObject({ version: 2, teams: true, reminderMinutes: 60 });
  });

  it("schedules without an invitation when the Outlook invitation is switched off, in person", async () => {
    readsWithOutlook(false);
    const post = vi.spyOn(api, "POST").mockResolvedValueOnce(ok(interview()));
    renderWithClient(<ApplicationInterviews applicationId="appl_1" closed={false} permissions={CALENDAR} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    await addInterviewer("usr_2");
    fireEvent.click(await screen.findByRole("button", { name: "Outlook invitation" }));
    fireEvent.click(screen.getByRole("radio", { name: "In person" }));
    fireEvent.change(screen.getByLabelText("Address"), { target: { value: "Level 12, Menara Maju" } });
    fireEvent.change(screen.getByLabelText("End time"), { target: { value: "90" } });
    fireEvent.click(screen.getByRole("button", { name: "Schedule" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
    expect(sentBody(post)).toMatchObject({ mode: "in_person", location: "Level 12, Menara Maju", durationMinutes: 90 });
  });

  it("keeps a changed interview's place as it was unless a new one is picked", async () => {
    reads([]);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(interview()));
    renderWithClient(<InterviewItem interview={interview({ location: null })} permissions={[...RECRUITER, "interviews:read", "interviews:schedule"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Change" }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Second interview" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ title: "Second interview", mode: "video", location: null, version: 1 });
  });

  it("will not send a template's unfilled fields, drops its instructions placeholder, and keeps it as instructions without an invitation", async () => {
    readsWithOutlook();
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(path === "/v1/email-drafts"
      ? ok({ to: [], subject: "Interview", body: "", missing: ["[Client contact]"],
        bodyHtml: "<p>Meet [Client contact] on {{interview.when}}.</p><p>{{interview.instructions}}</p>" })
      : ok(interview()))) as never);
    renderWithClient(<ApplicationInterviews applicationId="appl_1" candidateId="cand_1" closed={false} permissions={[...CALENDAR, "emails:send"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    await addInterviewer("usr_2");
    fireEvent.click(screen.getByRole("radio", { name: "Zoom" }));
    fireEvent.change(screen.getByLabelText("Zoom link"), { target: { value: "https://zoom.us/j/1" } });
    fireEvent.click(screen.getByRole("button", { name: "Template" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Interview invitation" }));
    await waitFor(() => { expect(document.querySelector(".rich-box")?.innerHTML).toContain("[Client contact]"); });
    expect(document.querySelector(".rich-box")?.innerHTML).not.toContain("{{interview.instructions}}");
    const draft = (post.mock.calls as unknown as [string, { body: Record<string, unknown> }][]).find(([p]) => p === "/v1/email-drafts")?.[1];
    expect(draft?.body).not.toHaveProperty("interviewId"); // filled from the composer, not the saved interview
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText(/Fill in \[Client contact\] first/)).toBeTruthy();
    const paths = () => (post.mock.calls as unknown as [string][]).map(([p]) => p);
    expect(paths()).not.toContain("/v1/applications/{applicationId}/interviews");
    // Filled in, and scheduled without the invitation: the message is kept as the instructions.
    const box = document.querySelector(".rich-box");
    if (box) { box.innerHTML = box.innerHTML.replace("[Client contact]", "Aina"); fireEvent.input(box); }
    fireEvent.click(screen.getByRole("button", { name: "Outlook invitation" }));
    fireEvent.click(screen.getByRole("button", { name: "Schedule" }));
    await waitFor(() => { expect(paths()).toContain("/v1/applications/{applicationId}/interviews"); });
    const created = (post.mock.calls as unknown as [string, { body: { instructions: string } }][]).find(([p]) => p === "/v1/applications/{applicationId}/interviews")?.[1];
    expect(created?.body.instructions).toMatch(/^Meet Aina on /);
  });

  it("asks for the Teams link when nothing would make the meeting", async () => {
    readsWithOutlook();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(interview()));
    renderWithClient(<ApplicationInterviews applicationId="appl_1" closed={false} permissions={CALENDAR} />);
    fireEvent.click(await screen.findByRole("button", { name: "Schedule interview" }));
    pickTime();
    await addInterviewer("usr_2");
    expect((await screen.findByRole("radio", { name: "Teams" })).getAttribute("aria-checked")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Outlook invitation" })); // no invitation: Outlook makes no meeting
    fireEvent.click(screen.getByRole("button", { name: "Schedule" }));
    expect(await screen.findByText("Paste the Teams meeting link, or send the Outlook invitation to make one.")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });

  it("keeps the room beside a video interview's link when it is changed", async () => {
    reads([]);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(interview()));
    renderWithClient(<InterviewItem interview={interview({ location: "Room 4B; https://zoom.us/j/123" })} permissions={[...RECRUITER, "interviews:read", "interviews:schedule"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Change" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ location: "Room 4B; https://zoom.us/j/123", mode: "video" });
  });

  it("shows which service an interview's link joins", async () => {
    reads([]);
    renderWithClient(<InterviewItem interview={interview({ location: "https://meet.google.com/abc-defg-hij" })} permissions={["interviews:read"]} />);
    expect((await screen.findByRole("link", { name: "Join Google Meet" })).getAttribute("href")).toBe("https://meet.google.com/abc-defg-hij");
  });

  it("asks interviewers for feedback once the interview has taken place", async () => {
    reads([]);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(interview()));
    renderWithClient(<InterviewItem interview={interview({ myFeedbackDue: true, feedbackHidden: 1 })} permissions={[...RECRUITER, "interviews:feedback"]} />);
    expect(await screen.findByText(/1 interviewer\(s\) gave feedback; you will see it once you give yours/)).toBeTruthy();
    fireEvent.click(await screen.findByLabelText("Yes"));
    fireEvent.change(screen.getByLabelText("Comments"), { target: { value: "Clear thinker." } });
    fireEvent.click(screen.getByRole("button", { name: "Save feedback" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ recommendation: "yes", comments: "Clear thinker." });
  });

  it("shows feedback, the meeting link, and no actions to viewers", async () => {
    reads([]);
    renderWithClient(<InterviewItem interview={interview({
      status: "completed", feedback: [{ userId: "usr_2", name: "Aina", recommendation: "strong_yes", comments: "Hire.", submittedAt: "2026-11-10T04:00:00Z" }],
    })} permissions={["interviews:read"]} />);
    expect(await screen.findByText(/Strong yes/)).toBeTruthy();
    expect(screen.getByRole("link", { name: "join" }).getAttribute("href")).toBe("https://meet.example/abc");
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    expect(screen.getByRole("button", { name: "Add to calendar" })).toBeTruthy();
  });

  it("emails the candidate an invitation filled from the interview", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => {
      if (path === "/v1/me") return Promise.resolve(ok(me));
      if (path === "/v1/email-banners") return Promise.resolve(ok({ items: [] }));
      return Promise.resolve(ok({ items: [{ id: "etpl_1", version: 1, name: "Interview invitation", subject: "s", body: "b", createdAt: "x", updatedAt: "x" }], placeholders: [] }));
    }) as never);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ to: ["siti@example.com"], subject: "Interview for Accountant", body: "Dear Siti", missing: [] }));
    renderWithClient(<InterviewItem interview={interview()} permissions={[...RECRUITER, "interviews:schedule", "emails:send"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Email candidate" }));
    await waitFor(() => { expect(post.mock.calls.some((c) => c[0] === "/v1/email-drafts")).toBe(true); });
    const calls = post.mock.calls as unknown as [string, { body: unknown }][];
    expect(calls.find((c) => c[0] === "/v1/email-drafts")?.[1].body).toMatchObject({ templateId: "etpl_1", candidateId: "cand_1", applicationId: "appl_1", interviewId: interview().id });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Subject").value).toBe("Interview for Accountant"); });
  });
});
