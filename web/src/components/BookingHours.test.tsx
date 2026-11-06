import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { BookingHours } from "./BookingHours";
import { BookingLinkForm } from "./CandidateBookings";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const schedule = {
  set: false, timeZone: "Asia/Kuala_Lumpur", bufferMinutes: 10, minNoticeHours: 12, maxDaysAhead: 14, calendar: null,
  hours: [1, 2, 3, 4, 5].map((day) => ({ day, start: "09:00", end: "17:00" })),
};

describe("booking hours", () => {
  it("saves the hours and the calendar address", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(schedule));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({
      ...schedule, set: true, calendar: { host: "calendar.google.com", status: "pending", error: null, checkedAt: null, busyCount: 0 },
    }));
    renderWithClient(<BookingHours />);
    expect(await screen.findByText(/Not saved yet/)).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "Sat" }));
    fireEvent.change(screen.getByLabelText("Sat to"), { target: { value: "12:00" } });
    fireEvent.change(screen.getByLabelText("Calendar address (.ics)"), { target: { value: "https://calendar.google.com/calendar/ical/x/private-1/basic.ics" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    const body = sentBody(put) as { hours: { day: number; end: string }[]; calendarUrl?: string };
    expect(body.hours.find((h) => h.day === 6)?.end).toBe("12:00");
    expect(body.calendarUrl).toBe("https://calendar.google.com/calendar/ical/x/private-1/basic.ics");
    expect(await screen.findByText("Reading…")).toBeTruthy();
  });

  it("leaves the calendar address alone when none is typed", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...schedule, set: true }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...schedule, set: true }));
    renderWithClient(<BookingHours />);
    fireEvent.click(await screen.findByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).not.toHaveProperty("calendarUrl");
  });
});

describe("booking hours while the calendar is read", () => {
  it("keep what is being typed when the status is refreshed", async () => {
    const pending = { ...schedule, set: true, calendar: { host: "calendar.google.com", status: "pending", error: null, checkedAt: null, busyCount: 0 } };
    vi.spyOn(api, "GET").mockResolvedValue(ok(pending));
    const { client } = renderWithClient(<BookingHours />);
    const gap = await screen.findByLabelText<HTMLInputElement>("Gap around meetings (minutes)");
    fireEvent.change(gap, { target: { value: "25" } });
    const before = client.getQueryState(["schedule", "me"])?.dataUpdatedAt ?? 0;
    await new Promise((r) => setTimeout(r, 5)); // a later refresh time
    await client.refetchQueries({ queryKey: ["schedule", "me"] }); // the polling refresh
    expect(client.getQueryState(["schedule", "me"])?.dataUpdatedAt).toBeGreaterThan(before);
    await new Promise((r) => setTimeout(r, 20)); // let it render
    expect(screen.getByLabelText<HTMLInputElement>("Gap around meetings (minutes)").value).toBe("25");
  });
});

describe("booking link in an email", () => {
  it("makes the link and hands back the text to insert", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "bkl_1", url: "https://ats.example/book/abc", durationMinutes: 45 }));
    const insert = vi.fn();
    renderWithClient(<BookingLinkForm candidateId="cand_1" onInsert={insert} onCancel={() => undefined} />);
    fireEvent.change(screen.getByLabelText("Length"), { target: { value: "45" } });
    fireEvent.change(screen.getByLabelText("Where (link, phone or address)"), { target: { value: " https://meet.google.com/x " } });
    fireEvent.click(screen.getByRole("button", { name: "Insert link" }));
    await waitFor(() => { expect(insert).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ candidateId: "cand_1", title: "Call with the recruiter", durationMinutes: 45, location: "https://meet.google.com/x" });
    expect(insert).toHaveBeenCalledWith("Please pick a time that suits you (45 minutes): https://ats.example/book/abc");
  });
});
