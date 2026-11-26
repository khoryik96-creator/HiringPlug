import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { clearToken, getToken, setToken } from "../auth";
import { ok } from "../test-utils";
import { SessionKeeper, tokenExpiry } from "./SessionKeeper";

const tokenEnding = (ms: number) => {
  const payload = btoa(JSON.stringify({ sub: "usr_1", exp: Math.floor(ms / 1000) })).replace(/=+$/, "");
  return `header.${payload}.signature`;
};

beforeEach(() => { vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] }); });
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  clearToken();
});

describe("staying signed in", () => {
  it("reads when a sign-in ends", () => {
    expect(tokenExpiry(tokenEnding(Date.UTC(2026, 10, 26, 12)))).toBe(Date.UTC(2026, 10, 26, 12));
    expect(tokenExpiry("not a token")).toBeNull();
  });

  it("renews quietly while the person is working", async () => {
    setToken(tokenEnding(Date.now() + 20 * 60_000));
    const renewed = tokenEnding(Date.now() + 12 * 3_600_000);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ token: renewed, expiresAt: "" }));
    render(<SessionKeeper onSignOut={vi.fn()} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
    expect(post).toHaveBeenCalledWith("/v1/auth/renew");
    expect(getToken()).toBe(renewed);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("counts down the last five minutes when they have been away, and keeps them signed in on request", async () => {
    setToken(tokenEnding(Date.now() + 40 * 60_000));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ token: tokenEnding(Date.now() + 12 * 3_600_000), expiresAt: "" }));
    const onSignOut = vi.fn();
    render(<SessionKeeper onSignOut={onSignOut} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(36 * 60_000); }); // away: no keys, no clicks
    expect(post).not.toHaveBeenCalled();
    expect(screen.getByRole("alertdialog").textContent).toContain("You will be signed out in 4:00");
    fireEvent.click(screen.getByRole("button", { name: "Stay signed in" }));
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
    expect(post).toHaveBeenCalledWith("/v1/auth/renew");
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(onSignOut).not.toHaveBeenCalled();
  });

  it("signs out when the time is up", async () => {
    setToken(tokenEnding(Date.now() + 3_000));
    const onSignOut = vi.fn();
    render(<SessionKeeper onSignOut={onSignOut} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(4_000); });
    expect(onSignOut).toHaveBeenCalled();
  });
});
