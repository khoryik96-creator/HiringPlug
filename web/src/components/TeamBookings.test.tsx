import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { hoursText, TeamBookings } from "./TeamBookings";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const weekdays = [1, 2, 3, 4, 5].map((day) => ({ day, start: "09:00", end: "17:00" }));

describe("everyone's booking set-up", () => {
  it("says weekly hours in a few words", () => {
    expect(hoursText(weekdays)).toBe("Mon–Fri 09:00–17:00");
    expect(hoursText([...weekdays, { day: 6, start: "10:00", end: "13:00" }])).toBe("Mon–Fri 09:00–17:00, Sat 10:00–13:00");
    expect(hoursText([{ day: 1, start: "09:00", end: "12:00" }, { day: 3, start: "09:00", end: "12:00" }])).toBe("Mon 09:00–12:00, Wed 09:00–12:00");
  });

  it("lists each person's hours, calendar status and booking links", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      { userId: "usr_1", name: "Aina", set: true, timeZone: "Asia/Kuala_Lumpur", hours: weekdays,
        calendar: { host: "calendar.google.com", status: "ok", error: null, checkedAt: new Date().toISOString(), busyCount: 4 },
        openLinks: 2, upcoming: 1, nextAt: "2026-11-17T02:00:00Z" },
      { userId: "usr_2", name: "Wei", set: false, timeZone: "Asia/Kuala_Lumpur", hours: [], calendar: null, openLinks: 0, upcoming: 0, nextAt: null },
    ] }));
    renderWithClient(<TeamBookings />);
    const aina = (await screen.findByText("Aina")).closest("tr") as HTMLElement;
    expect(within(aina).getByText(/Mon–Fri 09:00–17:00/)).toBeTruthy();
    expect(aina.textContent).toContain("✓ calendar.google.com · read today");
    expect(aina.textContent).toContain("2 waiting · 1 booked · next");
    const wei = screen.getByText("Wei").closest("tr") as HTMLElement;
    expect(wei.textContent).toBe("WeiNot set upNot connectedNone");
  });
});
