import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { BookingPage, bookingRoute, byDay } from "./BookingPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const TOKEN = "a".repeat(43);
const open = {
  companyName: "Contoh Recruitment", hostName: "Aina", title: "Call with Aina", durationMinutes: 30, location: null,
  timeZone: "Asia/Kuala_Lumpur", status: "open", startsAt: null, canChange: false,
  slots: ["2026-11-10T02:00:00Z", "2026-11-10T02:30:00Z", "2026-11-11T06:00:00Z"],
} as const;

describe("booking page", () => {
  it("is found at /book/<token> and #/book/<token> only", () => {
    expect(bookingRoute({ pathname: `/book/${TOKEN}`, hash: "" })).toBe(TOKEN);
    expect(bookingRoute({ pathname: "/", hash: `#/book/${TOKEN}` })).toBe(TOKEN);
    expect(bookingRoute({ pathname: "/book/short", hash: "" })).toBeNull();
    expect(bookingRoute({ pathname: "/candidates", hash: "#/candidates" })).toBeNull();
  });

  it("groups the free times by day", () => {
    expect(byDay([...open.slots]).map(([, l]) => l.length)).toEqual([2, 1]);
  });

  it("books the time picked", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(open));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({
      ...open, status: "booked", startsAt: open.slots[1], canChange: true, location: "https://meet.google.com/abc", slots: [],
    }));
    renderWithClient(<BookingPage token={TOKEN} />);
    expect(await screen.findByText("Call with Aina")).toBeTruthy();
    const times = screen.getAllByRole("radio").filter((r) => r.closest("[aria-label='Time']"));
    expect(times).toHaveLength(2);
    fireEvent.click(times[1] as HTMLElement);
    fireEvent.click(screen.getByRole("button", { name: /^Book / }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ startsAt: open.slots[1] });
    expect(await screen.findByText("You're booked")).toBeTruthy();
    expect(screen.getByRole("link", { name: "https://meet.google.com/abc" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Change the time" })).toBeTruthy();
  });

  it("says when the link was cancelled", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...open, status: "cancelled", slots: [] }));
    renderWithClient(<BookingPage token={TOKEN} />);
    expect(await screen.findByText(/This meeting was cancelled/)).toBeTruthy();
    expect(screen.queryByRole("radio")).toBeNull();
  });

  it("drops a picked time that was taken meanwhile", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(open));
    vi.spyOn(api, "POST").mockResolvedValue({ data: undefined, error: { error: { code: "BOOKING_SLOT_TAKEN", message: "That time is no longer free: pick another.", requestId: "r" } }, response: new Response(null, { status: 409 }) });
    renderWithClient(<BookingPage token={TOKEN} />);
    const first = (await screen.findAllByRole("radio")).filter((r) => r.closest("[aria-label='Time']"))[0] as HTMLElement;
    fireEvent.click(first);
    get.mockResolvedValue(ok({ ...open, slots: open.slots.slice(1) })); // 02:00 was taken
    fireEvent.click(screen.getByRole("button", { name: /^Book / }));
    expect(await screen.findByText(/no longer free/)).toBeTruthy();
    await waitFor(() => { expect(screen.getByRole("button", { name: "Pick a time" })).toHaveProperty("disabled", true); });
  });
});
