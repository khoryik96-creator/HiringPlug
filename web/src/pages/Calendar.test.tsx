import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Schemas } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { Calendar } from "./Calendar";
import { EventForm } from "../components/calendar/EventForm";
import { addDays, dateLabel, daysFor, localInput, sameDay, startDay, timeLabel, zonedInput, wallTime } from "../components/calendar/dates";
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
it("says that a Teams meeting's changed notes stay in HiringPlug", () => {
  const event = { id: "cev_1", version: 1, title: "Interview", notes: "", location: "", startsAt: "2026-11-18T01:00:00Z", endsAt: "2026-11-18T02:00:00Z", timeZone: "Asia/Kuala_Lumpur", allDay: false, attendees: [], teams: true } as unknown as Schemas["CalendarEvent"];
  renderWithClient(<EventForm event={event} date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={vi.fn()}/>);
  expect(screen.getByText(/Changed notes stay in HiringPlug/)).toBeTruthy();
  cleanup();
  renderWithClient(<EventForm event={{ ...event, teams: false }} date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={vi.fn()}/>);
  expect(screen.queryByText(/Changed notes stay in HiringPlug/)).toBeNull();
});
it("builds Monday-first grids that cover the whole month", () => {
  const days = daysFor(new Date(2026, 10, 18), "month"); expect(days).toHaveLength(42); expect(days[0]?.getDay()).toBe(1); expect(days.some((day) => sameDay(day, new Date(2026, 10, 30)))).toBe(true);
});

it("shows hourly appointments, opens an exact early slot, and advances one day", async () => {
  const today = startDay(new Date());
  const slot = (hours: number) => new Date(today.getFullYear(), today.getMonth(), today.getDate(), hours);
  const event: Schemas["CalendarEvent"] = {
    id: "cev_1", version: 1, title: "Morning interview", startsAt: slot(9).toISOString(), endsAt: slot(10).toISOString(),
    notes: "", location: "", timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone, allDay: false,
    teams: false, attendees: [], source: "local", sourceId: null, status: "active", syncStatus: "local",
    syncError: null, webUrl: null, joinUrl: null, editable: true,
  };
  const read = vi.spyOn(api, "GET").mockResolvedValue(ok({
    items: [event, { ...event, id: "cev_2", title: "Team offsite", allDay: true, startsAt: today.toISOString(), endsAt: addDays(today, 1).toISOString() },
      { ...event, id: "cev_3", title: "Overlapping phone screen" }],
    connection: disconnected,
  }));
  renderWithClient(<Calendar />);
  fireEvent.click(screen.getByRole("button", { name: /^Day$/ }));
  const day = within(await screen.findByRole("region", { name: "Day calendar" }));
  expect(day.getByText("All day")).toBeTruthy();
  expect(day.getByRole("button", { name: /Morning interview/ }).style.width).toContain("50%");
  expect(day.getByRole("button", { name: /Overlapping phone screen/ }).style.left).toBe("50%");
  fireEvent.click(day.getByRole("button", { name: `New appointment at ${timeLabel(slot(6).toISOString())}` }));
  expect(screen.getByLabelText<HTMLInputElement>("Start").value).toBe(localInput(slot(6)));
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  fireEvent.click(screen.getByRole("button", { name: "Next period" }));
  await waitFor(() => { expect(read).toHaveBeenLastCalledWith("/v1/calendar/events", {
    params: { query: { from: addDays(today, 1).toISOString(), to: addDays(today, 2).toISOString() } },
  }); });
});

it("shows seven hourly week columns and opens the selected day's slot", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], connection: disconnected }));
  renderWithClient(<Calendar />);
  fireEvent.click(screen.getByRole("button", { name: "Week" }));
  const week = within(await screen.findByRole("region", { name: "Week calendar" }));
  expect(week.getAllByRole("group")).toHaveLength(7);
  const selected = daysFor(new Date(), "week")[2];
  if (!selected) throw new Error("Missing Wednesday");
  selected.setHours(6);
  fireEvent.click(within(week.getByRole("group", { name: dateLabel(selected) })).getByRole("button", { name: `New appointment ${dateLabel(selected)} at ${timeLabel(selected.toISOString())}` }));
  expect(screen.getByLabelText<HTMLInputElement>("Start").value).toBe(localInput(selected));
});

it("preserves the event timezone and refuses ambiguous or missing DST wall times", () => {
  expect(wallTime("2026-11-18T09:00", "Asia/Kuala_Lumpur").toISOString()).toBe("2026-11-18T01:00:00.000Z");
  expect(zonedInput(new Date("2026-11-18T01:00:00Z"), "Asia/Kuala_Lumpur")).toBe("2026-11-18T09:00");
  expect(() => wallTime("2026-03-08T02:30", "America/New_York")).toThrow(/does not exist/);
  expect(() => wallTime("2026-11-01T01:30", "America/New_York")).toThrow(/occurs twice/);
});
