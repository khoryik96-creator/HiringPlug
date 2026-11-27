import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { type Schemas } from "../../api/client";
import { renderWithClient } from "../../test-utils";
import { dateLabel, timeLabel } from "./dates";
import { EventDetails } from "./EventDetails";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const invitation: Schemas["CalendarEvent"] = {
  id: "cev_1", version: 3, title: "Final interview with the panel", startsAt: "2026-12-02T02:00:00Z", endsAt: "2026-12-02T03:15:00Z",
  notes: "Bring two forms of ID.", location: "Level 12, Orbit Tower", timeZone: "UTC", allDay: false, teams: false,
  attendees: [{ email: "nadia@example.test" }, { email: "panel.guest@example.test" }], source: "interview_invitation",
  sourceId: "intv_1", status: "active", syncStatus: "synced", syncError: null, webUrl: null, joinUrl: null, editable: false,
  outlookChangedAt: "2026-11-24T06:30:00Z",
};
const none = () => undefined;

it("says when an interview invitation was changed in Outlook, and that Outlook's changes are taken in", () => {
  renderWithClient(<EventDetails event={invitation} onClose={none} onEdit={none} onChanged={none} />);
  const when = `${dateLabel(new Date(invitation.outlookChangedAt ?? ""))} ${timeLabel(invitation.outlookChangedAt ?? "")}`;
  expect(screen.getByText(`Changed in Outlook ${when}`)).toBeTruthy();
  expect(screen.getByText(/panel\.guest@example\.test/)).toBeTruthy();
  expect(screen.getByText("Changes made in Outlook are taken into the interview on the next refresh. Cancel it in HyPlug.")).toBeTruthy();
});

it("says nothing about Outlook changes for a meeting never changed there", () => {
  renderWithClient(<EventDetails event={{ ...invitation, source: "interview", outlookChangedAt: null }} onClose={none} onEdit={none} onChanged={none} />);
  expect(screen.queryByText(/Changed in Outlook/)).toBeNull();
  expect(screen.getByText("Change this meeting in its HyPlug interview or booking link.")).toBeTruthy();
});
