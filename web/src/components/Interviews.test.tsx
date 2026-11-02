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
    fireEvent.change(screen.getByLabelText("Date and time"), { target: { value: "2026-11-10T10:00" } });
    fireEvent.click(await screen.findByLabelText("Aina"));
    expect(screen.queryByLabelText("Vee")).toBeNull(); // viewers cannot interview
    fireEvent.click(screen.getByRole("button", { name: "Schedule" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Aina: Panel");
    expect(sentBody(post)).toMatchObject({ title: "First interview", mode: "video", interviewerIds: ["usr_2"], ignoreClashes: false });
    fireEvent.click(within(alert).getByRole("button", { name: "Schedule anyway" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post, 1)).toMatchObject({ ignoreClashes: true });
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
      return Promise.resolve(ok({ items: [{ id: "etpl_1", version: 1, name: "Interview invitation", subject: "s", body: "b", createdAt: "x", updatedAt: "x" }], placeholders: [] }));
    }) as never);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ to: ["siti@example.com"], subject: "Interview for Accountant", body: "Dear Siti", missing: [] }));
    renderWithClient(<InterviewItem interview={interview()} permissions={[...RECRUITER, "interviews:schedule", "emails:send"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Email candidate" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ templateId: "etpl_1", candidateId: "cand_1", applicationId: "appl_1", interviewId: interview().id });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Subject").value).toBe("Interview for Accountant"); });
  });
});
