import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { InterviewInvitation } from "./InterviewInvitation";

const interview = {
  id: "intv_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 2, applicationId: "app_1", candidateId: "cand_1",
  candidateName: "Nadia", jobId: "jo_1", jobTitle: "Engineer", title: "Technical interview",
  startsAt: "2026-12-15T03:00:00Z", durationMinutes: 45, mode: "video", location: null,
  instructions: null, status: "scheduled", interviewers: [], feedback: [], feedbackHidden: 0,
  myFeedbackDue: false, createdAt: "2026-11-18T00:00:00Z", updatedAt: "2026-11-18T00:00:00Z",
} satisfies Schemas["Interview"];

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("shows the recipient and sends only with the organizer's chosen Teams and reminder options", async () => {
  const state = {
    status: "none", canSend: true, teamsAvailable: true, teams: false, reminderMinutes: 0,
    recipient: "nadia@example.test", joinUrl: null, issue: null,
  } satisfies Schemas["InterviewInvitation"];
  const attach = { ...state, status: "sent", canSend: false, teams: true, joinUrl: "https://teams.microsoft.com/l/meetup-join/test" };
  const get = vi.spyOn(api, "GET").mockResolvedValueOnce(ok(state)).mockResolvedValue(ok(attach));
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...state, status: "pending", canSend: false }));
  renderWithClient(<InterviewInvitation interview={interview} />);
  expect(get).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Outlook invitation" }));
  expect(await screen.findByText("To nadia@example.test")).toBeTruthy();
  fireEvent.click(screen.getByLabelText("Microsoft Teams meeting"));
  fireEvent.change(screen.getByLabelText("Outlook reminder"), { target: { value: "30" } });
  fireEvent.click(screen.getByRole("button", { name: "Send Outlook invitation" }));
  await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
  expect(sentBody(post)).toEqual({ version: 2, teams: true, reminderMinutes: 30 });
  expect((await screen.findByRole("link", { name: "Join Teams" })).getAttribute("href")).toBe(attach.joinUrl);
});

it("starts with Teams ticked for an interview scheduled as a Teams meeting", async () => {
  const state = {
    status: "none", canSend: true, teamsAvailable: true, teams: false, reminderMinutes: 0,
    recipient: "nadia@example.test", joinUrl: null, issue: null,
  } satisfies Schemas["InterviewInvitation"];
  vi.spyOn(api, "GET").mockResolvedValue(ok(state));
  renderWithClient(<InterviewInvitation interview={{ ...interview, location: "Microsoft Teams Meeting" }} />);
  fireEvent.click(screen.getByRole("button", { name: "Outlook invitation" }));
  expect((await screen.findByLabelText<HTMLInputElement>("Microsoft Teams meeting")).checked).toBe(true);
  cleanup();
  renderWithClient(<InterviewInvitation interview={{ ...interview, location: "Room 4B; Microsoft Teams Meeting" }} />);
  fireEvent.click(screen.getByRole("button", { name: "Outlook invitation" }));
  expect((await screen.findByLabelText<HTMLInputElement>("Microsoft Teams meeting")).checked).toBe(true); // a room beside it
  cleanup();
  renderWithClient(<InterviewInvitation interview={{ ...interview, location: "https://zoom.us/j/1" }} />);
  fireEvent.click(screen.getByRole("button", { name: "Outlook invitation" }));
  expect((await screen.findByLabelText<HTMLInputElement>("Microsoft Teams meeting")).checked).toBe(false);
});
