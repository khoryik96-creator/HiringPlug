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
const found = (c: typeof edran | typeof edwin) => ({ ...c, employer: null, score: null });
const search = (text: string, more: Record<string, unknown>) => ({
  text, kind: "name", company: null, error: null, note: null, items: [], similar: [], more: false, ...more,
});

describe("dashboard search", () => {
  it("shows a box only for what the role may read", () => {
    renderWithClient(<QuickSearch permissions={["candidates:read", "jobs:read"]} />);
    expect(screen.getByRole("search", { name: "Search candidates" })).toBeTruthy();
    expect(screen.getByRole("search", { name: "Search jobs" })).toBeTruthy();
    expect(screen.queryByRole("search", { name: "Search contacts" })).toBeNull();
    expect(screen.queryByRole("search", { name: "Search companies" })).toBeNull();
  });

  it("finds candidates as you type, offers names alike, and Enter opens the list", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(() => Promise.resolve(ok({
      skipped: 0,
      searches: [search("edran", { items: [found(edran)], similar: [found(edran), { ...found(edwin), score: 0.78 }] })],
    })));
    renderWithClient(<QuickSearch permissions={RECRUITER} />);
    const input = screen.getByRole("searchbox", { name: "Search candidates" });

    fireEvent.change(input, { target: { value: "edran" } });
    const hits = await screen.findByRole("list", { name: "Candidates" });
    expect(within(hits).getByRole("link", { name: /Edran Zainal/ }).getAttribute("href")).toBe("#/candidates/cand_edran");
    expect(hits.textContent).toContain("DBA · Lenovo · edran@example.com");
    // Names alike, without repeating Edran, with how alike.
    const similar = await screen.findByRole("list", { name: "Similar candidates" });
    expect(within(similar).getAllByRole("link").map((a) => a.textContent)).toEqual(["Edwin Zain78% name match · +60 12 345 6789"]);
    expect(get).toHaveBeenCalledWith("/v1/candidates/find", { params: { query: { q: "edran" } } });

    fireEvent.submit(screen.getByRole("search", { name: "Search candidates" }));
    expect(window.location.hash).toBe("#/candidates?q=edran"); // a similar name too: the full list
  });

  it("runs several candidate searches at once, a block each, with Kano's messages", async () => {
    vi.spyOn(api, "GET").mockImplementation(() => Promise.resolve(ok({
      skipped: 1,
      searches: [
        search("edran - lenovo", { company: "lenovo", items: [{ ...found(edran), employer: { company: "Lenovo", title: "DBA", current: false } }] }),
        search("+60 12 345 6789", { kind: "phone", items: [found(edwin)], note: "No country code was typed." }),
        search("a@@b", { kind: "email", error: "Enter a complete email address, for example aaa@gmail.com." }),
      ],
    })));
    renderWithClient(<QuickSearch permissions={["candidates:read"]} />);
    fireEvent.change(screen.getByRole("searchbox", { name: "Search candidates" }), { target: { value: "edran - lenovo, +60 12 345 6789, a@@b, x, y, z, w" } });

    const first = await screen.findByRole("list", { name: "Candidates: edran - lenovo" });
    expect(first.textContent).toContain("DBA · Lenovo (worked there) · edran@example.com");
    expect(screen.getByRole("list", { name: "Candidates: +60 12 345 6789" }).textContent).toContain("Edwin Zain");
    expect(screen.getByText("No country code was typed.")).toBeTruthy();
    expect(screen.getByText("Enter a complete email address, for example aaa@gmail.com.")).toBeTruthy();
    expect(screen.getByText("1 more search was left out: six at a time.")).toBeTruthy();

    fireEvent.submit(screen.getByRole("search", { name: "Search candidates" }));
    expect(window.location.hash).toBe("", "several searches: Enter stays on the results");
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
