import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { NotificationBell, NotificationSettings } from "./Notifications";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const noContent = () => ({ data: undefined, error: undefined, response: new Response(null, { status: 204 }) }) as never;

function list(over: Partial<Schemas["NotificationList"]> = {}): Schemas["NotificationList"] {
  return {
    unseen: 1, morningEmail: false, mutedKinds: [], asOf: "2026-11-22T02:00:00Z",
    items: [
      { id: "applicant:app_1", kind: "applicant", title: "Siti Aminah applied", detail: "Accountant, Contoh Berhad", href: "#/candidates/cand_1", at: "2026-11-22T01:00:00Z", new: true },
      { id: "task:tsk_1", kind: "task", title: "Call Wei back", detail: "Overdue, due Fri 20 Nov", href: "#/", at: "2026-11-20T00:00:00Z", new: false },
    ],
    ...over,
  };
}

describe("NotificationBell", () => {
  it("shows how many are new, lists them, and marks them seen when opened", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(list()));
    const post = vi.spyOn(api, "POST").mockResolvedValue(noContent());
    renderWithClient(<NotificationBell />);
    const bell = await screen.findByLabelText("Notifications: 1 new");
    expect(within(bell).getByText("1")).toBeTruthy();

    fireEvent.click(bell);
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/notifications/seen");
    expect(sentBody(post)).toEqual({ upTo: "2026-11-22T02:00:00Z" }); // the list shown, not later
    const menu = screen.getByRole("dialog", { name: "Notifications" });
    const applicant = within(menu).getByText("Siti Aminah applied").closest("li");
    expect(applicant?.className).toContain("new"); // still marked while open
    expect(within(menu).getByText("Call Wei back").closest("a")?.getAttribute("href")).toBe("#/");
    await screen.findByTitle("Notifications"); // the count is gone
  });

  it("lights green the moment a CV is formatted, naming the candidate", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(list({ items: [
      { id: "cv_formatted:scv_1", kind: "cv_formatted", title: "Blind CV formatted: Siti Aminah", detail: "Ready under CVs on their page", href: "#/candidates/cand_1", at: "2026-11-22T01:30:00Z", new: true },
    ] })));
    vi.spyOn(api, "POST").mockResolvedValue(noContent());
    renderWithClient(<NotificationBell />);
    const bell = await screen.findByLabelText("Notifications: 1 new, a CV is formatted");
    expect(bell.className).toContain("cv-ready");
    fireEvent.click(bell);
    const menu = await screen.findByRole("dialog", { name: "Notifications" });
    expect(within(menu).getByText("CV ready")).toBeTruthy();
    expect(within(menu).getByText("Blind CV formatted: Siti Aminah").closest("a")?.getAttribute("href")).toBe("#/candidates/cand_1");
  });

  it("is marked while open, and closes by itself after 30 seconds", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(list({ unseen: 0, items: [] })));
    renderWithClient(<NotificationBell />);
    const bell = await screen.findByTitle("Notifications");
    vi.useFakeTimers();
    try {
      fireEvent.click(bell);
      await vi.advanceTimersByTimeAsync(10);
      expect(bell.getAttribute("aria-expanded")).toBe("true");
      await vi.advanceTimersByTimeAsync(29_000);
      expect(bell.closest("details")?.open).toBe(true);
      await vi.advanceTimersByTimeAsync(1_100);
      expect(bell.closest("details")?.open).toBe(false);
      expect(bell.getAttribute("aria-expanded")).toBe("false");
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not mark seen when nothing is new, and says when there is nothing", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(list({ unseen: 0, items: [] })));
    const post = vi.spyOn(api, "POST");
    renderWithClient(<NotificationBell />);
    fireEvent.click(await screen.findByTitle("Notifications"));
    expect(screen.getByText("Nothing needs you right now.")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });

  it("caps the count it shows", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(list({ unseen: 12 })));
    renderWithClient(<NotificationBell />);
    expect(within(await screen.findByLabelText("Notifications: 12 new")).getByText("9+")).toBeTruthy();
  });
});

describe("NotificationSettings", () => {
  it("offers a switch for each kind the user can get, and saves turning one off", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(list()));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ morningEmail: false, mutedKinds: ["task"] }));
    renderWithClient(<NotificationSettings permissions={["pipeline:read", "automations.tasks:read", "interviews:read"]} />);
    const tasks = await screen.findByRole("switch", { name: "Your tasks due today or overdue" });
    expect(screen.queryByRole("switch", { name: "Offers waiting for your approval" })).toBeNull(); // cannot approve offers
    expect(tasks).toHaveProperty("checked", true);

    fireEvent.click(tasks);
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(put.mock.calls[0]?.[0]).toBe("/v1/notifications/settings");
    expect(sentBody(put)).toEqual({ morningEmail: false, mutedKinds: ["task"] });
    expect(await screen.findByText("Saved.")).toBeTruthy();
  });

  it("turns the morning email on, keeping the kinds turned off", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(list({ mutedKinds: ["applicant"] })));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ morningEmail: true, mutedKinds: ["applicant"] }));
    renderWithClient(<NotificationSettings permissions={["pipeline:read"]} />);
    expect(await screen.findByRole<HTMLInputElement>("switch", { name: "New applicants on jobs you own" })).toHaveProperty("checked", false);
    fireEvent.click(screen.getByRole("switch", { name: /Email me this list at 8:00/ }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ morningEmail: true, mutedKinds: ["applicant"] });
  });

  it("says when the settings cannot be loaded, instead of vanishing", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(failure("INTERNAL", "Something went wrong.", undefined, 500));
    renderWithClient(<NotificationSettings permissions={["pipeline:read"]} />);
    expect(await screen.findByRole("button", { name: "Try again" })).toBeTruthy();
  });
});
