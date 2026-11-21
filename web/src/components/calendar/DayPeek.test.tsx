import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { ok, renderWithClient } from "../../test-utils";
import { DayPeek } from "./DayPeek";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const at = (h: number, m = 0) => new Date(2026, 10, 18, h, m);
const event = (id: string, title: string, from: Date, to: Date) => ({
  id, version: 1, title, startsAt: from.toISOString(), endsAt: to.toISOString(), notes: "", location: "", timeZone: "UTC", allDay: false,
  teams: false, attendees: [], source: "outlook", sourceId: null, status: "active", syncStatus: "synced", syncError: null, webUrl: null, joinUrl: null, editable: false,
});

it("shows the day with the new meeting on it, and says when it overlaps", async () => {
  const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [event("cev_1", "Client call", at(10), at(11)), event("cev_2", "Being changed", at(14), at(15))] }));
  renderWithClient(<DayPeek day={at(0)} start={at(10, 30)} end={at(11, 30)} title="First interview" skip="cev_2" />);
  expect(await screen.findByText("Client call")).toBeTruthy();
  expect(screen.getByText("First interview")).toBeTruthy();
  expect(screen.getByRole("status").textContent).toBe("Overlaps your calendar");
  expect(screen.queryByText("Being changed")).toBeNull(); // not shown against itself
  expect(get).toHaveBeenCalledWith("/v1/calendar/events", { params: { query: { from: at(0).toISOString(), to: new Date(2026, 10, 19).toISOString() } } });
});

it("says the user is free when nothing overlaps", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [event("cev_1", "Client call", at(10), at(11))] }));
  renderWithClient(<DayPeek day={at(0)} start={at(11)} end={at(12)} title="" />);
  expect((await screen.findByRole("status")).textContent).toBe("You are free then");
  expect(screen.getByText("New meeting")).toBeTruthy();
});
