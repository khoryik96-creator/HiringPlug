import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Schemas } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { Calendar } from "./Calendar";
import { EventForm } from "../components/calendar/EventForm";
import { daysFor, sameDay, zonedInput, wallTime } from "../components/calendar/dates";
const disconnected: Schemas["OutlookConnection"] = { connected: false, ready: false, status: "disconnected", callbackUrl: "http://app.test/v1/calendar/callback", version: 0, address: null, calendarName: null, lastSyncAt: null, error: null, teamsAvailable: false };
const connection: Schemas["OutlookConnection"] = { ...disconnected, connected: true, ready: true, status: "ok", address: "recruiter@agency.test", teamsAvailable: true };
afterEach(() => { cleanup(); vi.restoreAllMocks(); window.location.hash = ""; });
it("shows compact views and keeps Outlook unavailable until the app is configured", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], connection: disconnected })); renderWithClient(<Calendar/>);
  expect((await screen.findByRole<HTMLButtonElement>("button", { name: "Connect Outlook" })).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Week" })); expect(screen.getByRole("button", { name: "Week" }).getAttribute("aria-pressed")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Agenda" })); expect(await screen.findAllByText("No appointments")).toHaveLength(7);
});
it("finishes OAuth in the original signed-in session and removes the ticket from history", async () => {
  window.location.hash = "#/calendar?outlook=confirm&ticket=synthetic-one-time-ticket-0123456789";
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], connection })); const post = vi.spyOn(api, "POST").mockResolvedValue(ok(connection)); renderWithClient(<Calendar/>);
  await waitFor(() => { expect(post).toHaveBeenCalled(); }); expect(post.mock.calls[0]?.[0]).toBe("/v1/calendar/complete"); expect(window.location.hash).toBe("#/calendar");
});
it("makes attendee invitations explicit and highlights rejected fields", async () => {
  const post = vi.spyOn(api, "POST").mockResolvedValue(invalidField("attendees", "Enter valid email addresses."));
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={vi.fn()}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Interview" } }); fireEvent.change(screen.getByLabelText("Attendees"), { target: { value: "invalid" } });
  fireEvent.click(screen.getByRole("button", { name: "Create & send invitations" }));
  expect(await screen.findByText("Enter valid email addresses.")).toBeTruthy(); expect(screen.getByLabelText("Attendees").getAttribute("aria-invalid")).toBe("true");
  expect(sentBody(post)).toMatchObject({ title: "Interview", attendees: [{ email: "invalid" }] });
});
it("requires valid intervals before sending and uses local midnight for all-day appointments", async () => {
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({})); const onSaved = vi.fn();
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={disconnected} onClose={vi.fn()} onSaved={onSaved}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Day off" } }); fireEvent.click(screen.getByLabelText("All day"));
  expect(screen.getByLabelText<HTMLInputElement>("Start").value).toBe("2026-11-18T00:00"); expect(screen.getByLabelText<HTMLInputElement>("End").value).toBe("2026-11-19T00:00");
  fireEvent.click(screen.getByRole("button", { name: "Save appointment" })); await waitFor(() => { expect(onSaved).toHaveBeenCalled(); });
  expect(sentBody(post)).toMatchObject({ allDay: true, attendees: [] });
});
it("builds Monday-first grids that cover the whole month", () => {
  const days = daysFor(new Date(2026, 10, 18), "month"); expect(days).toHaveLength(42); expect(days[0]?.getDay()).toBe(1); expect(days.some((day) => sameDay(day, new Date(2026, 10, 30)))).toBe(true);
});

it("preserves the event timezone and refuses ambiguous or missing DST wall times", () => {
  expect(wallTime("2026-11-18T09:00", "Asia/Kuala_Lumpur").toISOString()).toBe("2026-11-18T01:00:00.000Z");
  expect(zonedInput(new Date("2026-11-18T01:00:00Z"), "Asia/Kuala_Lumpur")).toBe("2026-11-18T09:00");
  expect(() => wallTime("2026-03-08T02:30", "America/New_York")).toThrow(/does not exist/);
  expect(() => wallTime("2026-11-01T01:30", "America/New_York")).toThrow(/occurs twice/);
});
