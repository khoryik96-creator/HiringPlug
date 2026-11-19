import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "./api/client";
import { App, initials } from "./App";
import { setToken } from "./auth";
import { ok, RECRUITER, testCandidate, testFieldSettings, testMe } from "./test-utils";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
  localStorage.removeItem(`hp.calendarView.${testMe([]).user.id}`);
});

const alice = testCandidate({ id: "cand_aaaaaaaaaaaaaaaaaaaaaaaaaa", firstName: "Alice", fullName: "Alice Tan", email: "alice@example.com" });
const bob = testCandidate({ id: "cand_bbbbbbbbbbbbbbbbbbbbbbbbbb", firstName: "Bob", fullName: "Bob Lee", email: "bob@example.com" });

type Init = { params?: { path?: { candidateId?: string } } } | undefined;

describe("app", () => {
  it("opens another candidate with a fresh page, so an open form cannot save onto them", async () => {
    vi.spyOn(api, "GET").mockImplementation((path: string, init: Init) => {
      if (path === "/v1/me") return Promise.resolve(ok(testMe(RECRUITER)));
      if (path === "/v1/candidates/{candidateId}") {
        return Promise.resolve(ok(init?.params?.path?.candidateId === alice.id ? alice : bob));
      }
      if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: [] }));
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });
    setToken("test-token");

    window.location.hash = `#/candidates/${bob.id}`;
    render(<App />);
    await screen.findByRole("heading", { name: "Bob Lee" }, {timeout: 5000});
    window.location.hash = `#/candidates/${alice.id}`;
    await screen.findByRole("heading", { name: "Alice Tan" });
    fireEvent.click(screen.getByRole("button", { name: "Edit profile" }));
    expect(screen.getByLabelText<HTMLInputElement>("First name").value).toBe("Alice");

    // Straight to Bob (already loaded, e.g. with the Back button): his page, not Alice's open form.
    window.location.hash = `#/candidates/${bob.id}`;
    await screen.findByRole("heading", { name: "Bob Lee" });
    expect(screen.queryByRole("form", { name: "Edit profile" })).toBeNull();
    expect(within(screen.getByRole("region", {name: "Candidate"})).getByText("bob@example.com")).toBeTruthy();
  });
});

describe("account button", () => {
  it("shows the first letters of the first two names", () => {
    expect(initials("Demo recruiter")).toBe("DR");
    expect(initials("  Siti  Nur Aisyah ")).toBe("SN");
    expect(initials("Ali")).toBe("A");
    expect(initials("")).toBe("?");
  });
});

describe("top bar", () => {
  const signedIn = (permissions: Parameters<typeof testMe>[0], hash = "#/account") => {
    vi.spyOn(api, "GET").mockImplementation((path: string) => {
      if (path === "/v1/me") return Promise.resolve(ok(testMe(permissions)));
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      return Promise.resolve(ok({ items: [] }));
    });
    setToken("test-token");
    window.location.hash = hash;
    render(<App />);
  };

  it("shows HiringPlug's version (the VERSION file) beside its name, so anyone can tell which update runs", async () => {
    signedIn(RECRUITER);
    const version = await screen.findByTitle("HiringPlug version");
    expect(version.textContent).toBe(`v${__APP_VERSION__}`);
    expect(__APP_VERSION__).toMatch(/^\d+\.\d+\.\d+$/);
  });

  it("has one Settings button, leading to every part of Settings the user may open", async () => {
    signedIn([...RECRUITER, "settings:manage", "aiproviders:manage", "identity.team:manage"]);
    const button = await screen.findByRole("link", { name: "Settings" });
    expect(button.getAttribute("href")).toBe("#/settings");
    expect(button.getAttribute("aria-current")).toBe("page"); // the account is part of Settings
    const menu = () => screen.getByRole("navigation", { name: "Settings" });
    // The profile is fetched again on each page load (an earlier test may have left another one cached).
    await waitFor(() => {
      expect(within(menu()).getAllByRole("link").map((a) => a.textContent)).toEqual(["General", "Salary", "Dropdowns & categories", "Industries", "AI provider", "Team", "System", "Your account", "My dropdowns", "LinkedIn companion"]);
    });
    expect(within(menu()).getByRole("link", { name: "Your account" }).getAttribute("aria-current")).toBe("page");
    expect(screen.queryByText("Admin")).toBeNull();
  });

  it("opens the first part a recruiter may change, and leaves out the company's settings", async () => {
    signedIn(RECRUITER, "#/settings");
    const menu = () => screen.getByRole("navigation", { name: "Settings" });
    await waitFor(() => { expect(within(menu()).getAllByRole("link").map((a) => a.textContent)).toEqual(["AI provider", "Your account", "My dropdowns", "LinkedIn companion"]); });
    expect(within(menu()).getByRole("link", { name: "Your account" }).getAttribute("aria-current")).toBe("page");
  });

  it("keeps Leads, Reports and Import under More, which names the one open", async () => {
    signedIn([...RECRUITER, "leads:read", "reports:read"], "#/reports");
    const nav = await screen.findByRole("navigation", { name: "Main" });
    const more = await within(nav).findByLabelText("More pages");
    await waitFor(() => { expect(more.textContent).toContain("Reports"); }); // on the Reports page
    expect(within(nav).getByRole("link", { name: "Leads" }).closest(".menu-list"), "not on the bar itself").toBeTruthy();
    fireEvent.click(more);
    const list = nav.querySelector(".menu-list") as HTMLElement;
    expect(within(list).getAllByRole("link").map((a) => a.textContent)).toEqual(["Leads", "Reports", "Import"]);
    expect(within(list).getByRole("link", { name: "Reports" }).getAttribute("aria-current")).toBe("page");
  });

  it("does not open a part the user may not change", async () => {
    signedIn(RECRUITER, "#/team");
    await screen.findByRole("navigation", { name: "Main" });
    await waitFor(() => { expect(screen.queryByRole("navigation", { name: "Settings" })).toBeNull(); });
  });

  it("places Outlook beside Calendar in the header for permitted users", async () => {
    signedIn([...RECRUITER, "calendars:use", "emails:send"]);
    const icon = await screen.findByRole("link", { name: /^Calendar$/ });
    const outlook = await screen.findByRole("link", { name: "Outlook" });
    expect(icon.getAttribute("href")).toBe("#/calendar");
    expect(outlook.getAttribute("href")).toBe("#/outlook");
    expect(outlook.nextElementSibling).toBe(icon);
    expect(icon.nextElementSibling).toBe(screen.getByRole("button", { name: "Dock calendar" }));
    const nav = within(screen.getByRole("navigation", { name: "Main" }));
    expect(nav.queryByRole("link", { name: /^(Calendar|Outlook)$/ })).toBeNull();
  });

  it("docks the calendar without leaving an open candidate form, and remembers the view", async () => {
    const permissions = [...RECRUITER, "calendars:use"] as const;
    vi.spyOn(api, "GET").mockImplementation((path: string) => {
      if (path === "/v1/me") return Promise.resolve(ok(testMe([...permissions])));
      if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(alice));
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      if (path === "/v1/calendar/events") return Promise.resolve(ok({ items: [] }));
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });
    setToken("test-token");
    const candidateHash = `#/candidates/${alice.id}`;
    window.location.hash = candidateHash;
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Edit profile" }));
    fireEvent.change(screen.getByLabelText("First name"), { target: { value: "Unsaved name" } });
    const toggle = screen.getByRole("button", { name: "Dock calendar" });
    fireEvent.click(toggle);
    const dock = await screen.findByRole("complementary", { name: "Calendar dock" });
    const day = await within(dock).findByRole("button", { name: "Day" });
    expect(day.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(dock).getByRole("button", { name: "Week" }));
    fireEvent.click(within(dock).getByRole("button", { name: "Today" }));
    expect(within(dock).getByRole("button", { name: "Week" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.keyDown(dock, { key: "Escape" });
    expect(screen.queryByRole("complementary", { name: "Calendar dock" })).toBeNull();
    expect(document.activeElement).toBe(toggle);
    expect(window.location.hash).toBe(candidateHash);
    expect(screen.getByLabelText<HTMLInputElement>("First name").value).toBe("Unsaved name");
    window.location.hash = "#/calendar";
    expect((await screen.findByRole("button", { name: "Week" })).getAttribute("aria-pressed")).toBe("true");
  });

  it("ends the session on the server when signing out", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(undefined));
    signedIn(RECRUITER);
    fireEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/auth/sign-out"); });
    expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy();
  });
});
