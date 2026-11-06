import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "./api/client";
import { App, initials } from "./App";
import { setToken } from "./auth";
import { ok, RECRUITER, testCandidate, testMe } from "./test-utils";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
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
    await screen.findByRole("heading", { name: "Bob Lee" });
    window.location.hash = `#/candidates/${alice.id}`;
    await screen.findByRole("heading", { name: "Alice Tan" });
    fireEvent.click(screen.getByRole("button", { name: "Edit profile" }));
    expect(screen.getByLabelText<HTMLInputElement>("First name").value).toBe("Alice");

    // Straight to Bob (already loaded, e.g. with the Back button): his page, not Alice's open form.
    window.location.hash = `#/candidates/${bob.id}`;
    await screen.findByRole("heading", { name: "Bob Lee" });
    expect(screen.queryByRole("form", { name: "Edit profile" })).toBeNull();
    expect(screen.getByText("bob@example.com")).toBeTruthy();
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
  const signedIn = (permissions: Parameters<typeof testMe>[0]) => {
    vi.spyOn(api, "GET").mockImplementation((path: string) => {
      if (path === "/v1/me") return Promise.resolve(ok(testMe(permissions)));
      return Promise.resolve(ok({ items: [] }));
    });
    setToken("test-token");
    window.location.hash = "#/account";
    render(<App />);
  };

  it("puts the pages for running the account under Admin, only for those allowed", async () => {
    signedIn([...RECRUITER, "settings:manage", "identity.team:manage"]);
    const admin = await screen.findByText("Admin");
    const menu = admin.closest("details");
    expect(menu).toBeTruthy();
    const links = Array.from(menu?.querySelectorAll("a") ?? []).map((a) => a.textContent);
    expect(links).toEqual(["Settings", "Team"]);
    expect(screen.getByRole("navigation", { name: "Main" }).textContent).not.toContain("Settings");
  });

  it("shows no Admin menu to a recruiter", async () => {
    signedIn(RECRUITER);
    await screen.findByRole("navigation", { name: "Main" });
    // The profile is fetched again on each page load (an earlier test may have left another one cached).
    await waitFor(() => { expect(screen.queryByText("Admin")).toBeNull(); });
  });
});
