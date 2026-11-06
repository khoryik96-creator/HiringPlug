import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { hashParam } from "../hashQuery";
import { ok, RECRUITER, renderWithClient } from "../test-utils";
import { QuickSearch } from "./QuickSearch";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
});

type Init = { params?: { query?: { q?: string; similar?: boolean } } } | undefined;

const edran = { id: "cand_edran", fullName: "Edran Zainal", currentTitle: "DBA", currentCompany: "Lenovo", email: "edran@example.com", phone: null };
const edwin = { id: "cand_edwin", fullName: "Edwin Zain", currentTitle: null, currentCompany: null, email: null, phone: "+60 12 345 6789" };

describe("dashboard search", () => {
  it("shows a box only for what the role may read", () => {
    renderWithClient(<QuickSearch permissions={["candidates:read", "jobs:read"]} />);
    expect(screen.getByRole("search", { name: "Search candidates" })).toBeTruthy();
    expect(screen.getByRole("search", { name: "Search jobs" })).toBeTruthy();
    expect(screen.queryByRole("search", { name: "Search contacts" })).toBeNull();
    expect(screen.queryByRole("search", { name: "Search companies" })).toBeNull();
  });

  it("finds candidates as you type, offers names alike, and Enter opens the only match", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation((_path: string, init: Init) => {
      const query = init?.params?.query;
      if (query?.similar) return Promise.resolve(ok({ items: [edran, edwin], nextCursor: null }));
      return Promise.resolve(ok({ items: query?.q === "edran" ? [edran] : [], nextCursor: null }));
    });
    renderWithClient(<QuickSearch permissions={RECRUITER} />);
    const input = screen.getByRole("searchbox", { name: "Search candidates" });

    fireEvent.change(input, { target: { value: "edran" } });
    const hits = await screen.findByRole("list", { name: "Candidates" });
    expect(within(hits).getByRole("link", { name: /Edran Zainal/ }).getAttribute("href")).toBe("#/candidates/cand_edran");
    expect(hits.textContent).toContain("DBA · Lenovo · edran@example.com");
    // Few found: names alike are offered too, without repeating Edran.
    const similar = await screen.findByRole("list", { name: "Similar candidates" });
    expect(within(similar).getAllByRole("link").map((a) => a.textContent)).toEqual(["Edwin Zain+60 12 345 6789"]);
    expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { q: "edran", limit: 5, similar: true } } });

    fireEvent.submit(screen.getByRole("search", { name: "Search candidates" }));
    expect(window.location.hash).toBe("#/candidates?q=edran"); // a similar name too: the full list
  });

  it("Enter opens the one match, or the full list", async () => {
    vi.spyOn(api, "GET").mockImplementation((_path: string, init: Init) => Promise.resolve(ok({
      items: init?.params?.query?.q === "maju" ? [{ id: "jo_1", title: "Finance Manager", companyName: "Maju Foods", location: "KL", status: "open" }] : [],
      nextCursor: null,
    })));
    renderWithClient(<QuickSearch permissions={["jobs:read"]} />);
    const input = screen.getByRole("searchbox", { name: "Search jobs" });
    fireEvent.change(input, { target: { value: "maju" } });
    const hits = await screen.findByRole("list", { name: "Jobs" });
    expect(hits.textContent).toContain("Finance ManagerOpen");
    fireEvent.submit(screen.getByRole("search", { name: "Search jobs" }));
    expect(window.location.hash).toBe("#/jobs/jo_1");

    fireEvent.change(input, { target: { value: "nothing here" } });
    expect(await screen.findByText("No jobs match “nothing here”.")).toBeTruthy();
    fireEvent.submit(screen.getByRole("search", { name: "Search jobs" }));
    expect(window.location.hash).toBe("#/jobs?q=nothing%20here");
  });

  it("list pages read the search from the address", () => {
    expect(hashParam("q", "#/candidates?q=edran%20lenovo")).toBe("edran lenovo");
    expect(hashParam("q", "#/candidates")).toBe("");
  });
});
