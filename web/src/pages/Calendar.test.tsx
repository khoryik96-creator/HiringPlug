import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Schemas } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { Calendar } from "./Calendar";
import { EventForm } from "../components/calendar/EventForm";
import { clockRange } from "../components/calendar/CalendarTimeGrid";
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
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Interview" } });
  const people = screen.getByLabelText("Attendees");
  fireEvent.change(people, { target: { value: "invalid, hr@maju.test hr@maju.test" } }); fireEvent.keyDown(people, { key: "Enter" });
  expect(screen.getByRole("alert").textContent).toBe("Not an email address: invalid"); // stays typed, named
  expect(screen.getAllByText("hr@maju.test")).toHaveLength(1); // a chip, once, as in Outlook
  fireEvent.submit(screen.getByRole("form", { name: "New appointment" }));
  expect(post).not.toHaveBeenCalled();
  fireEvent.change(people, { target: { value: "" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(await screen.findByText("Enter valid email addresses.")).toBeTruthy(); expect(screen.getByLabelText("Attendees").getAttribute("aria-invalid")).toBe("true");
  expect(sentBody(post)).toMatchObject({ title: "Interview", attendees: [{ email: "hr@maju.test" }] });
});
it("requires valid intervals before sending and uses local midnight for all-day appointments", async () => {
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({})); const onSaved = vi.fn();
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={disconnected} onClose={vi.fn()} onSaved={onSaved}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Day off" } }); fireEvent.click(screen.getByLabelText("All day"));
  expect(screen.getByLabelText<HTMLInputElement>("Start date").value).toBe("2026-11-18"); expect(screen.getByLabelText<HTMLInputElement>("End date").value).toBe("2026-11-18");
  fireEvent.change(screen.getByLabelText("End date"), { target: { value: "2026-11-19" } }); // the last day, as people say it
  fireEvent.click(screen.getByRole("button", { name: "Save" })); await waitFor(() => { expect(onSaved).toHaveBeenCalled(); });
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  expect(sentBody(post)).toMatchObject({ allDay: true, attendees: [], startsAt: wallTime("2026-11-18T00:00", zone).toISOString(), endsAt: wallTime("2026-11-20T00:00", zone).toISOString() });
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
  expect(screen.getByLabelText<HTMLInputElement>("Start date").value).toBe(localInput(slot(6)).slice(0, 10));
  expect(screen.getByLabelText<HTMLInputElement>("Start time").value).toBe(localInput(slot(6)).slice(11, 16));
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
  expect(screen.getByLabelText<HTMLInputElement>("Start date").value).toBe(localInput(selected).slice(0, 10));
  expect(screen.getByLabelText<HTMLInputElement>("Start time").value).toBe(localInput(selected).slice(11, 16));
});

it("preserves the event timezone and refuses ambiguous or missing DST wall times", () => {
  expect(wallTime("2026-11-18T09:00", "Asia/Kuala_Lumpur").toISOString()).toBe("2026-11-18T01:00:00.000Z");
  expect(zonedInput(new Date("2026-11-18T01:00:00Z"), "Asia/Kuala_Lumpur")).toBe("2026-11-18T09:00");
  expect(() => wallTime("2026-03-08T02:30", "America/New_York")).toThrow(/does not exist/);
  expect(() => wallTime("2026-11-01T01:30", "America/New_York")).toThrow(/occurs twice/);
});

it("puts a Google Meet or Zoom link in the location beside the place, and checks it is one", async () => {
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({})); const onSaved = vi.fn();
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={onSaved}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Client catch-up" } });
  fireEvent.click(screen.getByRole("radio", { name: "Google Meet" }));
  expect(screen.getByRole("link", { name: "New Google Meet ↗" }).getAttribute("href")).toBe("https://meet.google.com/new");
  fireEvent.change(screen.getByLabelText("Google Meet link"), { target: { value: "https://zoom.us/j/123" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByText("This is not a Google Meet link.")).toBeTruthy();
  expect(post).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Google Meet link"), { target: { value: "https://meet.google.com/abc-defg-hij" } });
  fireEvent.change(screen.getByLabelText("Location"), { target: { value: "Room 3" } });
  fireEvent.change(screen.getByLabelText("End time"), { target: { value: "30" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" })); await waitFor(() => { expect(onSaved).toHaveBeenCalled(); });
  expect(sentBody(post)).toMatchObject({ location: "Room 3; https://meet.google.com/abc-defg-hij", teams: false,
    startsAt: new Date(2026, 10, 18, 9).toISOString(), endsAt: new Date(2026, 10, 18, 9, 30).toISOString() });
});
it("lets Outlook make a Teams meeting, and opens a saved link back in its own service", async () => {
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({})); const onSaved = vi.fn();
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={onSaved}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Sync" } });
  fireEvent.click(screen.getByRole("radio", { name: "Teams" }));
  expect(screen.getByText(/Outlook adds the Teams link/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Save" })); await waitFor(() => { expect(onSaved).toHaveBeenCalled(); });
  expect(sentBody(post)).toMatchObject({ teams: true, location: "" });
  cleanup();
  const event = { id: "cev_1", version: 1, title: "Sync", notes: "", location: "Room 3; https://us02web.zoom.us/j/81", startsAt: "2026-11-18T01:00:00Z", endsAt: "2026-11-18T02:00:00Z", timeZone: "Asia/Kuala_Lumpur", allDay: false, attendees: [], teams: false } as unknown as Schemas["CalendarEvent"];
  renderWithClient(<EventForm event={event} date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={vi.fn()}/>);
  expect(screen.getByRole("radio", { name: "Zoom" }).getAttribute("aria-checked")).toBe("true");
  expect(screen.getByLabelText<HTMLInputElement>("Zoom link").value).toBe("https://us02web.zoom.us/j/81");
  expect(screen.getByLabelText<HTMLInputElement>("Location").value).toBe("Room 3");
});
it("keeps an opened appointment's meeting as it was unless one is picked", async () => {
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({})); const onSaved = vi.fn();
  const event = { id: "cev_1", version: 3, title: "Old call", notes: "", location: "Microsoft Teams Meeting", startsAt: "2026-11-18T01:00:00Z", endsAt: "2026-11-18T02:00:00Z", timeZone: "Asia/Kuala_Lumpur", allDay: false, attendees: [], teams: false } as unknown as Schemas["CalendarEvent"];
  renderWithClient(<EventForm event={event} date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={onSaved}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Old call, moved" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" })); await waitFor(() => { expect(onSaved).toHaveBeenCalled(); });
  expect(sentBody(put)).toMatchObject({ title: "Old call, moved", teams: false, location: "Microsoft Teams Meeting", version: 3 });
});
it("asks for the Teams link when Outlook cannot make the meeting", async () => {
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={{ ...connection, teamsAvailable: false }} onClose={vi.fn()} onSaved={vi.fn()}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Sync" } });
  fireEvent.click(screen.getByRole("radio", { name: "Teams" }));
  expect(screen.queryByText(/Outlook adds the Teams link/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByText("Paste the Teams meeting link.")).toBeTruthy();
  expect(post).not.toHaveBeenCalled();
});
it("shows three meetings a month day and opens the day for the rest", async () => {
  const today = startDay(new Date());
  const at = (h: number) => new Date(today.getFullYear(), today.getMonth(), today.getDate(), h).toISOString();
  const base = { version: 1, notes: "", location: "", timeZone: "UTC", allDay: false, teams: false, attendees: [], source: "local", sourceId: null, status: "active", syncStatus: "local", syncError: null, webUrl: null, joinUrl: null, editable: true };
  const items = [8, 9, 10, 11, 12].map((h) => ({ ...base, id: `cev_${String(h)}`, title: `Meeting at ${String(h)}`, startsAt: at(h), endsAt: at(h + 1) }));
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items, connection: disconnected }));
  renderWithClient(<Calendar />);
  fireEvent.click(screen.getByRole("button", { name: "Month" }));
  expect(await screen.findByRole("button", { name: /Meeting at 10/ })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /Meeting at 11/ })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "+2 more" }));
  expect(screen.getByRole("button", { name: "Day" }).getAttribute("aria-pressed")).toBe("true");
  expect(await screen.findByRole("button", { name: /Meeting at 12/ })).toBeTruthy();
});
it("writes a meeting's time as Outlook does", () => {
  const day = (h: number, m = 0) => new Date(2026, 10, 18, h, m).toISOString();
  const am = new Date(2026, 0, 1, 9).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  if (/AM$/.test(am)) { // in a 12-hour locale
    expect(clockRange(day(9), day(9, 30))).toBe("9:00 – 9:30 AM");
    expect(clockRange(day(11, 30), day(13))).toBe("11:30 AM – 1:00 PM");
  } else {
    expect(clockRange(day(9), day(9, 30))).toContain("–");
  }
});
it("invites optional attendees and sends the notes formatted, with the signature", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ bodyHtml: "<p>Best, Demo</p>", version: 1 }));
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({})); const onSaved = vi.fn();
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={onSaved}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Client catch-up" } });
  const to = screen.getByLabelText("Attendees");
  fireEvent.change(to, { target: { value: "hr@maju.test" } }); fireEvent.keyDown(to, { key: "Enter" });
  fireEvent.click(screen.getByRole("button", { name: "Optional" }));
  const cc = screen.getByLabelText("Optional attendees");
  fireEvent.change(cc, { target: { value: "boss@maju.test" } }); fireEvent.keyDown(cc, { key: "Enter" });
  await waitFor(() => { expect(screen.getByRole("button", { name: "Signature" }).getAttribute("aria-pressed")).toBe("true"); });
  fireEvent.click(screen.getByRole("button", { name: "Send" })); await waitFor(() => { expect(onSaved).toHaveBeenCalled(); });
  const body = sentBody(post) as { attendees: unknown; notes: string; notesHtml: string };
  expect(body.attendees).toEqual([{ email: "hr@maju.test" }, { email: "boss@maju.test", optional: true }]);
  expect(body.notesHtml).toContain('<div data-signature="true"><p>Best, Demo</p></div>');
  expect(body.notes).toBe(""); // the signature is not the appointment's notes
});
it("leaves an opened appointment's notes alone unless they are written in", async () => {
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({})); const onSaved = vi.fn();
  const event = { id: "cev_1", version: 2, title: "Sync", notes: "Agenda", notesHtml: "<p><b>Agenda</b></p>", location: "", startsAt: "2026-11-18T01:00:00Z", endsAt: "2026-11-18T02:00:00Z", timeZone: "Asia/Kuala_Lumpur", allDay: false, attendees: [], teams: false } as unknown as Schemas["CalendarEvent"];
  renderWithClient(<EventForm event={event} date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={onSaved}/>);
  fireEvent.change(screen.getByLabelText("End time"), { target: { value: "30" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" })); await waitFor(() => { expect(onSaved).toHaveBeenCalled(); });
  const body = sentBody(put) as Record<string, unknown>;
  expect(body.notes).toBe("Agenda");
  expect(body).not.toHaveProperty("notesHtml"); // left out: kept, and Outlook's body is not replaced
});
it("shows why formatted notes were refused", async () => {
  vi.spyOn(api, "POST").mockResolvedValue(invalidField("notesHtml", "is too long: shorten the notes"));
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={disconnected} onClose={vi.fn()} onSaved={vi.fn()}/>);
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Long" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByText("is too long: shorten the notes")).toBeTruthy();
});
it("shows the signature's banner as it is written, and says the invitation leaves it out", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(new Uint8Array([137, 80, 78, 71]), { status: 200 }));
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/email-banners"
    ? ok({ items: [{ id: "ebn_01jbz6w0m0k8q9x2r3s4t5v6w7", name: "Logo", contentType: "image/png", width: 600, height: 120 }] })
    : ok({ bodyHtml: '<p>Demo</p><img data-banner-id="ebn_01jbz6w0m0k8q9x2r3s4t5v6w7" alt="Logo">', version: 1 }))) as never);
  renderWithClient(<EventForm date={new Date(2026, 10, 18, 9)} connection={connection} onClose={vi.fn()} onSaved={vi.fn()}/>);
  expect(await screen.findByText(/left out of the Outlook invitation/)).toBeTruthy();
  expect(document.querySelector(".rich-box img[data-banner-id]")).toBeTruthy();
});
