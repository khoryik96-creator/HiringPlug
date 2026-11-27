import { QueryClient } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "./api/client";
import { App, initials } from "./App";
import { clearToken, getToken, setToken } from "./auth";
import { ok, RECRUITER, testCandidate, testFieldSettings, testMe } from "./test-utils";

afterEach(() => {
  cleanup();
  clearToken();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
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
  it("drops a late Apollo sign-in response after the recruiter switches accounts", async () => {
    if (typeof HTMLDialogElement.prototype.showModal !== "function") {
      Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
      Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value(this: HTMLDialogElement) { this.open = false; } });
    }
    let resolveLate: (result: never) => void = () => { throw Error("Expected a pending request"); };
    const late = new Promise<never>((resolve) => { resolveLate = resolve; });
    const me = testMe(["companies:read", "contacts.apollo:use"]);
    const company = {
      id: "comp_aaaaaaaaaaaaaaaaaaaaaaaaaa", name: "Fixture company", type: "client",
      linkedinUrl: "https://www.linkedin.com/company/fixture/", version: 1,
    };
    vi.spyOn(api, "GET").mockImplementation((path: string) => {
      switch (path) {
        case "/v1/me": return Promise.resolve(ok({ ...me, user: { ...me.user, name: getToken() === "test-first" ? "First recruiter" : "Second recruiter" } }));
        case "/v1/companies/{companyId}": return Promise.resolve(ok(company));
        case "/v1/companies/{companyId}/hr-scan": return Promise.resolve(ok({ scan: null }));
        case "/v1/contacts/apollo": return Promise.resolve(ok({ state: "disconnected" }));
        case "/v1/settings/fields": return Promise.resolve(ok(testFieldSettings()));
      }
      return Promise.resolve(ok({ items: [] }));
    });
    const post = vi.spyOn(api, "POST").mockImplementation((path: string) => {
      switch (path) {
        case "/v1/contacts/apollo": return late;
        case "/v1/auth/sign-in": return Promise.resolve(ok({ token: "test-second" }));
        default: return Promise.resolve(ok(undefined));
      }
    });
    const openApollo = async () => {
      fireEvent.click(await screen.findByRole("tab", { name: "Find HR" }));
      fireEvent.click(await screen.findByRole("button", { name: "Apollo" }));
      return screen.findByRole("button", { name: "Connect Apollo" });
    };

    const mounted = vi.spyOn(QueryClient.prototype, "mount");
    setToken("test-first");
    window.location.hash = `#/companies/${company.id}`;
    render(<App />);
    fireEvent.click(await openApollo());
    const firstSession = mounted.mock.contexts.at(-1);
    expect(firstSession).toBeInstanceOf(QueryClient);
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/contacts/apollo", { body: { action: "start" } }); });
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    const signIn = await screen.findByRole("form", { name: "Sign in" });
    fireEvent.change(within(signIn).getByLabelText("Email"), { target: { value: "second@example.test" } });
    fireEvent.change(within(signIn).getByLabelText("Password"), { target: { value: "fixture-password" } });
    fireEvent.submit(signIn);
    await screen.findByRole("link", { name: "Your account: Second recruiter" });
    await openApollo();
    expect(mounted.mock.contexts.at(-1)).not.toBe(firstSession);
    await act(async () => {
      resolveLate(ok({ state: "pending", authorizationUrl: "https://app.apollo.io/#/oauth/authorize?fixture=first-user" }));
      await late;
      await new Promise((done) => { setTimeout(done, 0); });
    });
    expect(screen.getByRole("button", { name: "Connect Apollo" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: /Open Apollo/ })).toBeNull();
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

  it("makes the covered app inert when Calendar opens as a small-screen dialog", async () => {
    vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
    signedIn([...RECRUITER, "calendars:use"]);
    const toggle = await screen.findByRole("button", { name: "Dock calendar" });
    fireEvent.click(toggle);
    const panel = screen.getByRole("dialog", { name: "Calendar dock" });
    expect(panel.getAttribute("aria-modal")).toBe("true");
    expect(toggle.closest("header")?.hasAttribute("inert")).toBe(true);
    expect(document.querySelector(".shell-page")?.hasAttribute("inert")).toBe(true);
    fireEvent.keyDown(panel, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Calendar dock" })).toBeNull();
    await waitFor(() => { expect(document.activeElement).toBe(toggle); });
  });

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
      expect(within(menu()).getAllByRole("link", { hidden: true }).map((a) => a.textContent)).toEqual(["Your account", "My dropdowns", "Dropdowns & categories", "Industries", "Salary", "General", "Team", "AI provider", "LinkedIn companion", "System"]);
    });
    expect(within(menu()).getByRole("link", { name: "Your account" }).getAttribute("aria-current")).toBe("page");
    expect(screen.queryByText("Admin")).toBeNull();
  });

  it("opens the first part a recruiter may change, and leaves out the company's settings", async () => {
    signedIn(RECRUITER, "#/settings");
    const menu = () => screen.getByRole("navigation", { name: "Settings" });
    await waitFor(() => { expect(within(menu()).getAllByRole("link", { hidden: true }).map((a) => a.textContent)).toEqual(["Your account", "My dropdowns", "AI provider", "LinkedIn companion"]); });
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

  it("opens the pages from the Menu button (phones), and closes it on going to one", async () => {
    signedIn(RECRUITER, "#/candidates");
    const button = await screen.findByRole("button", { name: "Menu" });
    const bar = button.closest("header") as HTMLElement;
    expect(button.getAttribute("aria-expanded")).toBe("false");
    expect(button.getAttribute("aria-controls")).toBe(screen.getByRole("navigation", { name: "Main" }).id);
    fireEvent.click(button);
    expect(bar.classList.contains("menu-open")).toBe(true);
    expect(screen.getByRole("button", { name: "Close menu" }).getAttribute("aria-expanded")).toBe("true");
    act(() => {
      window.location.hash = "#/jobs";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    await waitFor(() => { expect(bar.classList.contains("menu-open")).toBe(false); });
    expect(screen.getByRole("button", { name: "Menu" }).getAttribute("aria-expanded")).toBe("false");
  });

  it("does not open a part the user may not change", async () => {
    signedIn(RECRUITER, "#/team");
    await screen.findByRole("navigation", { name: "Main" });
    await waitFor(() => { expect(screen.queryByRole("navigation", { name: "Settings" })).toBeNull(); });
  });

  it("has Help (?) in the header for everyone, just before Settings", async () => {
    signedIn(RECRUITER);
    const help = await screen.findByRole("link", { name: "Help" });
    expect(help.getAttribute("href")).toBe("#/help");
    expect(help.nextElementSibling?.getAttribute("href")).toBe("#/settings");
    expect(screen.getByRole("link", { name: "Settings" }).getAttribute("title"), "a tooltip when only its gear shows").toBe("Settings");
  });

  it("places Outlook beside Calendar in the header for permitted users", async () => {
    signedIn([...RECRUITER, "calendars:use", "emails:send"]);
    const icon = await screen.findByRole("link", { name: /^Calendar$/ });
    const outlook = await screen.findByRole("link", { name: "Outlook" });
    expect(icon.getAttribute("href")).toBe("#/calendar");
    expect(outlook.getAttribute("href")).toBe("#/outlook");
    expect(outlook.nextElementSibling).toBe(screen.getByRole("button", { name: "Dock mail" }));
    expect(outlook.nextElementSibling?.nextElementSibling).toBe(icon);
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

  it("docks the mail beside a record, one dock at a time, and keeps the page as it was", async () => {
    const permissions = [...RECRUITER, "calendars:use", "emails:send"] as const;
    vi.spyOn(api, "GET").mockImplementation((path: string) => {
      if (path === "/v1/me") return Promise.resolve(ok(testMe([...permissions])));
      if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(alice));
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      if (path === "/v1/calendar/events") return Promise.resolve(ok({ items: [] }));
      if (path === "/v1/outlook/mail") {
        return Promise.resolve(ok({
          connected: true, enabled: true, syncing: false, error: null, nextOffset: null,
          folders: [{ id: "in", name: "Inbox", kind: "inbox", parentId: null, unread: 2, total: 9 }],
          items: [],
        }));
      }
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });
    setToken("test-token");
    const candidateHash = `#/candidates/${alice.id}`;
    window.location.hash = candidateHash;
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Edit profile" }));
    fireEvent.change(screen.getByLabelText("First name"), { target: { value: "Unsaved name" } });
    fireEvent.click(screen.getByRole("button", { name: "Dock calendar" }));
    await screen.findByRole("complementary", { name: "Calendar dock" });
    const toggle = screen.getByRole("button", { name: "Dock mail" });
    fireEvent.click(toggle);
    const dock = await screen.findByRole("complementary", { name: "Mail dock" });
    expect(screen.queryByRole("complementary", { name: "Calendar dock" })).toBeNull();
    const folder = await within(dock).findByRole("combobox", { name: "Folder" });
    expect(within(folder).getByRole("option", { name: "Inbox (2)" })).toBeTruthy();
    expect(within(dock).getByRole("link", { name: "Open full mail" }).getAttribute("href")).toBe("#/outlook");
    expect(window.location.hash).toBe(candidateHash);
    expect(screen.getByLabelText<HTMLInputElement>("First name").value).toBe("Unsaved name");
    fireEvent.click(within(dock).getByRole("button", { name: "Close mail dock" }));
    expect(screen.queryByRole("complementary", { name: "Mail dock" })).toBeNull();
  });

  it("ends the session on the server when signing out", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(undefined));
    signedIn(RECRUITER);
    fireEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/auth/sign-out"); });
    expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy();
  });
});
