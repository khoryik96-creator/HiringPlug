import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Permission } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Help } from "./Help";

const RECRUITER: Permission[] = ["candidates:read", "intake:create", "candidates:create", "jobs:read", "jobs:create", "emails:send",
  "pipeline:read", "folders:use", "notifications:read", "matching:create"];
const VIEWER: Permission[] = ["candidates:read", "jobs:read", "pipeline:read"];

beforeEach(() => {
  window.location.hash = "#/help";
  vi.spyOn(api, "GET").mockResolvedValue(ok({ guideDismissed: false }));
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const topicNames = () => within(screen.getByRole("list", { name: "Help topics" })).getAllByRole("link").map((a) => a.querySelector("strong")?.textContent);

describe("the help page", () => {
  it("lists the topics the role can use", () => {
    renderWithClient(<Help permissions={RECRUITER} topicId="" />);
    expect(topicNames()).toContain("Add and import candidates");
    expect(topicNames()).toContain("WhatsApp");
    cleanup();
    renderWithClient(<Help permissions={VIEWER} topicId="" />);
    expect(topicNames()).not.toContain("Add and import candidates");
    expect(topicNames()).not.toContain("WhatsApp");
    expect(topicNames()[0]).toBe("Getting started");
  });

  it("searches as you type, keeps the search in the address and opens the best topic on Enter", () => {
    renderWithClient(<Help permissions={RECRUITER} topicId="" />);
    const box = screen.getByRole("searchbox", { name: "Search help" });
    fireEvent.change(box, { target: { value: "excel spreadsheet" } });
    const results = within(screen.getByRole("region", { name: "Search results" }));
    expect(results.getByRole("status").textContent).toBe("1 topic");
    expect(results.getByText(/takes a CSV file or an Excel file/)).toBeTruthy();
    expect(window.location.hash).toBe("#/help?q=excel%20spreadsheet");
    expect(results.getByRole("link", { name: "Add and import candidates" }).getAttribute("href")).toBe("#/help/add-candidates?q=excel%20spreadsheet");
    fireEvent.submit(box.closest("form") as HTMLFormElement);
    expect(window.location.hash).toBe("#/help/add-candidates?q=excel%20spreadsheet");
  });

  it("says when nothing matches, and still offers the topics", () => {
    renderWithClient(<Help permissions={RECRUITER} topicId="" />);
    fireEvent.change(screen.getByRole("searchbox", { name: "Search help" }), { target: { value: "spaceship" } });
    expect(screen.getByRole("status").textContent).toContain("Nothing found for “spaceship”");
    expect(topicNames().length).toBeGreaterThan(5);
  });

  it("starts from the search in the address (coming back from a topic)", () => {
    window.location.hash = "#/help?q=folders";
    renderWithClient(<Help permissions={RECRUITER} topicId="" />);
    expect((screen.getByRole("searchbox", { name: "Search help" }) as HTMLInputElement).value).toBe("folders");
    expect(screen.getByRole("link", { name: "Folders" })).toBeTruthy();
  });

  it("shows a topic with links to its screens, and only the parts the role can use", () => {
    window.location.hash = "#/help/search?q=boolean";
    renderWithClient(<Help permissions={VIEWER} topicId="search" />);
    expect(screen.getByRole("heading", { name: "Search for candidates", level: 2 })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Boolean search" })).toBeTruthy();
    expect(screen.queryByRole("region", { name: "AI search" })).toBeNull();
    expect(screen.getByRole("link", { name: "Candidates →" }).getAttribute("href")).toBe("#/candidates");
    expect(screen.queryByRole("link", { name: "AI search →" })).toBeNull();
    expect(screen.getByRole("link", { name: "← All help topics" }).getAttribute("href")).toBe("#/help?q=boolean");
  });

  it("does not show a topic the role cannot use", () => {
    renderWithClient(<Help permissions={VIEWER} topicId="whatsapp" />);
    expect(screen.getByRole("heading", { name: "Not found" })).toBeTruthy();
  });

  it("brings back the getting-started checklist for someone who hid it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ guideDismissed: true }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ guideDismissed: false }));
    renderWithClient(<Help permissions={RECRUITER} topicId="" />);
    fireEvent.click(await screen.findByRole("button", { name: "Show the getting-started checklist" }));
    await waitFor(() => { expect(window.location.hash).toBe("#/"); });
    expect(sentBody(put)).toEqual({ guideDismissed: false });
  });
});
