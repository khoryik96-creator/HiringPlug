import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "./api/client";
import { App } from "./App";
import { clearToken, setToken } from "./auth";
import { failure, ok, testCandidate, testFieldSettings, testMe } from "./test-utils";

const me = testMe(["candidates:read"]);
const owner = `${me.tenant.id}:${me.user.id}`;
const tabsKey = `hp.openTabs:${owner}`;
const saved = JSON.stringify([{ href: "#/candidates/cand_aaaaaaaaaaaaaaaaaaaaaaaaaa", kind: "candidate", label: "Alice Tan" }]);

afterEach(() => {
  cleanup();
  clearToken();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.removeItem(tabsKey);
});

function serve(meAnswer: unknown) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/me" ? meAnswer
      : path === "/v1/settings/fields" ? ok(testFieldSettings())
        : path === "/v1/candidates/{candidateId}" ? ok(testCandidate({ id: "cand_aaaaaaaaaaaaaaaaaaaaaaaaaa", firstName: "Alice", lastName: "Tan", fullName: "Alice Tan" }))
          : ok({ items: [] }),
  )) as never);
  vi.spyOn(api, "POST").mockResolvedValue(ok(undefined));
}

describe("open-record tabs and signing out", () => {
  it("keeps them when the sign-in lapses", async () => {
    localStorage.setItem(tabsKey, saved);
    serve(failure("UNAUTHENTICATED", "Sign in again.", undefined, 401));
    setToken("expired");
    window.location.hash = "#/dashboard";
    render(<App />);
    expect(await screen.findByRole("form", { name: "Sign in" })).toBeTruthy();
    expect(localStorage.getItem(tabsKey)).toBe(saved);
  });

  it("forgets them on Sign out", async () => {
    localStorage.setItem(tabsKey, saved);
    serve(ok(me));
    setToken("valid");
    window.location.hash = "#/dashboard";
    render(<App />);
    expect(await screen.findByRole("link", { name: /Alice Tan/ })).toBeTruthy(); // their tab
    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    expect(await screen.findByRole("form", { name: "Sign in" })).toBeTruthy();
    expect(localStorage.getItem(tabsKey)).toBeNull();
  });
});

describe("a new browser tab", () => {
  class Recorder {
    static posted: unknown[] = [];
    onmessage: ((e: MessageEvent) => void) | null = null;
    postMessage(m: unknown) { Recorder.posted.push(m); }
    close() { /* nothing to close */ }
  }

  it("asks the other tabs for their sign-in, but never on a public page", async () => {
    vi.stubGlobal("BroadcastChannel", Recorder);
    serve(ok(me));
    Recorder.posted = [];
    window.location.hash = `#/invite/${"a".repeat(64)}`; // an invitation link: public
    const view = render(<App />);
    await new Promise((r) => setTimeout(r, 20));
    expect(Recorder.posted).toEqual([]);
    view.unmount();
    window.location.hash = "#/candidates";
    render(<App />);
    expect(screen.getByRole("status").textContent).toBe("Opening…");
    await waitFor(() => { expect(Recorder.posted).toEqual([{ kind: "ask" }]); });
    expect(await screen.findByRole("form", { name: "Sign in" }, { timeout: 2000 })).toBeTruthy(); // nobody answered
  });
});
