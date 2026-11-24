import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  for (const kept of ["meaning", "sort", "openAs"]) localStorage.removeItem(`hiringplug.candidates.${kept}`);
  window.location.hash = "";
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const summary = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Siti Hajar", email: null, phone: null, currentTitle: "Account Executive",
  currentCompany: null, location: null, workAuthorizationStatus: null, tags: [], updatedAt: "2026-11-01T00:00:00Z",
  availability: null, availabilitySince: null, noticePeriod: null, currentSalary: null, expectedSalary: null, salaryRestricted: false,
  workAuthorizationBasis: null, nationality: null, residenceCountry: null, source: null, industry: null,
};
const wei = { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Wei Tan", currentTitle: "Finance Executive" };

/** The page's reads: what a word search finds (Siti Hajar, unless given), whether this server can search by meaning, and the rest. */
function answer({ words = () => [summary], available = true }: { words?: (q: string) => unknown[]; available?: boolean } = {}) {
  return ((path: string, init?: { params?: { query?: { q?: string } } }) => {
    if (path === "/v1/candidates") return Promise.resolve(ok({ items: words(init?.params?.query?.q ?? ""), nextCursor: null }));
    if (path === "/v1/meaning/status") return Promise.resolve(ok({ available, read: 3, total: 3 }));
    if (path === "/v1/me/preferences") return Promise.resolve(ok({ candidateColumns: null }));
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: [] }));
    return Promise.resolve(ok({ tags: [], skills: [], industries: [], specializations: [], certifications: [] }));
  }) as never;
}

const understood = { role: "accounts executive", family: "accounting", level: 2, levelName: "Executive", describes: false };
const found = {
  candidate: summary, matchedIn: [],
  why: { score: 0.87, strength: "strong", title: "Account Executive", titleWhen: "now", family: "accounting", level: "Executive", words: [] },
};
/** The search by meaning's answer: those found, best match first. */
const ranked = (items: unknown[] = [found]) => ok({ items, nextCursor: null, meaning: { status: "ready", read: 3, total: 3, understood } });

/** Switches Meaning on and searches those words by meaning. */
async function searchByMeaning(words: string) {
  fireEvent.click(screen.getByRole("button", { name: /Meaning/ }));
  fireEvent.change(screen.getByLabelText("Search candidates by meaning"), { target: { value: words } });
  await screen.findByText(/best match first/);
}

describe("searching candidates by meaning", () => {
  it("sends the words to be read, says so meanwhile, then ranks with why", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer());
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(ok({ items: [], nextCursor: null, meaning: { status: "reading" } }))
      .mockResolvedValue(ranked());
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByText("Siti Hajar");
    fireEvent.click(screen.getByRole("button", { name: /Meaning/ }));
    expect(localStorage.getItem("hiringplug.candidates.meaning")).toBe("1"); // remembered on this device
    fireEvent.change(screen.getByLabelText("Search candidates by meaning"), { target: { value: "accounts executive" } });

    expect(await screen.findByText(/the way a recruiter would/)).toBeTruthy();
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ meaning: "accounts executive", limit: 100 });
    expect(screen.queryByText("No one matches by meaning.")).toBeNull(); // not while the words are read

    const line = await screen.findByText(/whatever their titles say/, {}, { timeout: 3000 }); // asked again a moment later
    expect(line.textContent).toBe("Searching for accounts executive (accounting), executive level first, whatever their titles say. Best matches first.");
    // In a Match column beside the name, not a line across the other columns.
    expect(screen.getByRole("columnheader", { name: "Match" })).toBeTruthy();
    const cell = document.querySelector("td.col-match .why-line");
    expect(cell?.textContent).toBe("Strong matchas Account Executive (now)accounting · level: executive"); // the field on a line of its own
    expect(cell?.querySelector(".why-field")?.textContent).toBe("accounting · level: executive");
    expect(document.querySelector("td[colspan]")).toBeNull();
    expect(screen.getByText(/1 candidate, best match first/)).toBeTruthy();
  });

  it("offers a search by meaning when the words find no one, on a server that can", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(answer({ words: (q) => (q ? [] : [summary]) }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [], nextCursor: null, meaning: { status: "reading" } }));
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByText("Siti Hajar");
    expect(get, "asked only when the offer could show").not.toHaveBeenCalledWith("/v1/meaning/status");
    fireEvent.change(screen.getByLabelText("Search candidates"), { target: { value: "talent acquisition" } });
    fireEvent.click(await screen.findByRole("button", { name: "Search “talent acquisition” by meaning" }));
    await waitFor(() => { expect(post.mock.calls.some((c) => JSON.stringify(c).includes("\"meaning\":\"talent acquisition\""))).toBe(true); });
    expect(screen.getByRole("button", { name: /Meaning/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("offers no search by meaning for words that found someone (a name), nor on a server without it", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(answer());
    renderWithClient(<CandidateList />);
    await screen.findByText("Siti Hajar");
    fireEvent.change(screen.getByLabelText("Search candidates"), { target: { value: "Siti Hajar" } });
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { q: "Siti Hajar" } } }); });
    expect(await screen.findByText("1 candidate")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /by meaning/ })).toBeNull();
    expect(get).not.toHaveBeenCalledWith("/v1/meaning/status");
    cleanup();

    vi.spyOn(api, "GET").mockImplementation(answer({ words: (q) => (q ? [] : [summary]), available: false }));
    renderWithClient(<CandidateList />);
    await screen.findByText("Siti Hajar");
    fireEvent.change(screen.getByLabelText("Search candidates"), { target: { value: "talent acquisition" } });
    expect(await screen.findByText("No candidates match.")).toBeTruthy();
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/meaning/status"); });
    await new Promise((r) => setTimeout(r, 50)); // the server's answer is in
    expect(screen.queryByRole("button", { name: /by meaning/ })).toBeNull();
  });

  it("searches a name given in the address (from the dashboard) by words, keeping Meaning as it was kept", async () => {
    localStorage.setItem("hiringplug.candidates.meaning", "1");
    window.location.hash = "#/candidates?q=edran";
    const get = vi.spyOn(api, "GET").mockImplementation(answer());
    const post = vi.spyOn(api, "POST").mockResolvedValue(ranked());
    renderWithClient(<CandidateList />);
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { q: "edran" } } }); });
    expect(screen.getByLabelText<HTMLInputElement>("Search candidates").value).toBe("edran");
    expect(screen.getByRole("button", { name: /Meaning/ }).getAttribute("aria-pressed")).toBe("false");
    expect(post).not.toHaveBeenCalled();
    expect(localStorage.getItem("hiringplug.candidates.meaning"), "the switch kept on this device is left alone").toBe("1");
    fireEvent.click(screen.getByRole("button", { name: /Meaning/ })); // it can still be searched by meaning
    await waitFor(() => { expect(sentBody(post)).toMatchObject({ meaning: "edran" }); });
    cleanup();

    window.location.hash = "#/candidates";
    renderWithClient(<CandidateList />); // opened from the menu: by meaning, as kept
    expect(screen.getByRole("button", { name: /Meaning/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("reads AND, OR, NOT and quotes as words when searching by meaning, and says they work in the word search", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer());
    const post = vi.spyOn(api, "POST").mockResolvedValue(ranked());
    renderWithClient(<CandidateList />);
    await screen.findByText("Siti Hajar");
    fireEvent.click(screen.getByRole("button", { name: /Meaning/ }));
    const box = screen.getByLabelText("Search candidates by meaning");
    fireEvent.change(box, { target: { value: '"talent acquisition" -agency' } });
    const hint = screen.getByText(/work in the word search/);
    expect(hint.textContent).toBe("Search by meaning reads your words as a recruiter would; AND, OR, NOT and quotes work in the word search.");
    expect(box.getAttribute("aria-describedby")).toBe(hint.id);
    expect(screen.queryByText("Boolean"), "no Boolean badge").toBeNull();
    expect(document.querySelector(".bool-mirror, .bool-rule"), "nothing in blue").toBeNull();
    await waitFor(() => { expect(sentBody(post)).toMatchObject({ meaning: '"talent acquisition" -agency' }); });
    expect((sentBody(post) as { boolean?: string }).boolean).toBeUndefined();

    fireEvent.click(screen.getByRole("button", { name: /Meaning/ })); // by words, the rules are read: and shown
    expect(screen.getByText("Boolean")).toBeTruthy();
    expect(screen.queryByText(/work in the word search/)).toBeNull();
  });

  it("ranks best match first: no sort, no sorting headers; the kept order is back with the words", async () => {
    localStorage.setItem("hiringplug.candidates.sort", "expected_salary_asc");
    vi.spyOn(api, "GET").mockImplementation(answer());
    const post = vi.spyOn(api, "POST").mockResolvedValue(ranked());
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByText("Siti Hajar");
    expect(screen.getByRole("columnheader", { name: "Expected salary" }).getAttribute("aria-sort")).toBe("ascending");
    await searchByMeaning("accounts executive");
    for (const name of ["Name", "Notice period", "Current salary", "Expected salary"]) {
      const header = screen.getByRole("columnheader", { name });
      expect(header.hasAttribute("aria-sort"), `${name}: claims no order`).toBe(false);
      expect(header.querySelector("button"), `${name}: does not sort`).toBeNull();
    }
    expect(screen.queryByRole("button", { name: "Back to newest first" })).toBeNull();
    expect(post).toHaveBeenCalledTimes(1);
    expect((sentBody(post) as { sort?: string }).sort).toBeUndefined();

    fireEvent.click(screen.getByRole("button", { name: /Meaning/ })); // by words again
    expect(screen.getByRole("columnheader", { name: "Expected salary" }).getAttribute("aria-sort")).toBe("ascending");
    expect(screen.getByRole("button", { name: "Back to newest first" })).toBeTruthy();
  });

  it("exports those the search by meaning found, not everyone; a word search as before", async () => {
    localStorage.setItem("hiringplug.candidates.sort", "name"); // a kept order: no part of a search by meaning
    vi.spyOn(api, "GET").mockImplementation(answer());
    vi.spyOn(api, "POST").mockResolvedValue(ranked());
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(new Response("x", { status: 200, headers: { "Content-Disposition": 'attachment; filename="Candidates.csv"' } })));
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("URL", class extends URL { static override createObjectURL = vi.fn().mockReturnValue("blob:x"); static override revokeObjectURL = vi.fn(); });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    const exported = async () => {
      fetch.mockClear();
      fireEvent.click(screen.getByRole("menuitem", { name: "CSV" }));
      await waitFor(() => { expect(fetch).toHaveBeenCalled(); });
      await waitFor(() => { expect(screen.getByLabelText("Export candidates").textContent).not.toContain("Exporting"); });
      const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
      expect(path).toBe("/v1/candidates/export");
      return (JSON.parse(init.body as string) as { search: Record<string, unknown> }).search;
    };
    renderWithClient(<CandidateList canExport />);
    await screen.findByText("Siti Hajar");
    await searchByMeaning("accounts executive");
    const byMeaning = await exported();
    expect(byMeaning).toMatchObject({ meaning: "accounts executive" });
    expect(byMeaning.sort, "best match first").toBeUndefined();
    expect(byMeaning.q).toBeUndefined();

    fireEvent.click(screen.getByRole("button", { name: /Meaning/ })); // by words
    await screen.findByRole("button", { name: "Back to newest first" });
    const byWords = await exported();
    expect(byWords).toMatchObject({ q: "accounts executive", sort: "name" });
    expect(byWords.meaning).toBeUndefined();
  });

  it("in Full resume, starts again at the best match when Meaning is switched on, and says why each was found", async () => {
    localStorage.setItem("hiringplug.candidates.openAs", "resume");
    const get = vi.spyOn(api, "GET").mockImplementation(answer({ words: () => [summary, wei] }));
    const siti = { ...found, why: { ...found.why, passage: "Full set of accounts for a bank" } };
    vi.spyOn(api, "POST").mockResolvedValue(ranked([siti, { candidate: wei, matchedIn: [], why: { ...found.why, strength: "good", title: "Finance Executive" } }]));
    renderWithClient(<CandidateList />);
    fireEvent.change(await screen.findByLabelText("Search candidates"), { target: { value: "accounts executive" } });
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { q: "accounts executive" } } }); });
    fireEvent.click(await screen.findByRole("button", { name: /Wei Tan/ }));
    expect(screen.getByRole("region", { name: "Resume of Wei Tan" })).toBeTruthy();
    expect(document.querySelector(".pane-why"), "a word search: no why").toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Meaning/ })); // the same words, by meaning
    const pane = await screen.findByRole("region", { name: "Resume of Siti Hajar" }); // the best match, not the resume left open
    expect(pane.querySelector(".pane-why")?.textContent).toBe("Strong matchas Account Executive (now) · accounting · level: executiveFull set of accounts for a bank");
    expect(screen.getByRole("button", { name: /Siti Hajar/ }).textContent).toContain("Strong match");
    expect(screen.getByRole("button", { name: /Wei Tan/ }).textContent).toContain("Good match");
  });
});
