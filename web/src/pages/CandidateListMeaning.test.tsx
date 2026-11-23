import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  localStorage.removeItem("hiringplug.candidates.meaning");
  vi.restoreAllMocks();
});

const summary = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Siti Hajar", email: null, phone: null, currentTitle: "Account Executive",
  currentCompany: null, location: null, workAuthorizationStatus: null, tags: [], updatedAt: "2026-11-01T00:00:00Z",
  availability: null, availabilitySince: null, noticePeriod: null, currentSalary: null, expectedSalary: null, salaryRestricted: false,
  workAuthorizationBasis: null, nationality: null, residenceCountry: null, source: null, industry: null,
};

function answer() {
  return ((path: string) => {
    if (path === "/v1/candidates") return Promise.resolve(ok({ items: [summary], nextCursor: null }));
    if (path === "/v1/me/preferences") return Promise.resolve(ok({ candidateColumns: null }));
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    return Promise.resolve(ok({ tags: [], skills: [], industries: [], specializations: [], certifications: [] }));
  }) as never;
}

const understood = { role: "accounts executive", family: "accounting", level: 2, levelName: "Executive", describes: false };
const found = {
  candidate: summary, matchedIn: [],
  why: { score: 0.87, strength: "strong", title: "Account Executive", titleWhen: "now", family: "accounting", level: "Executive", words: [] },
};

describe("searching candidates by meaning", () => {
  it("sends the words to be read, says so meanwhile, then ranks with why", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer());
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(ok({ items: [], nextCursor: null, meaning: { status: "reading" } }))
      .mockResolvedValue(ok({ items: [found], nextCursor: null, meaning: { status: "ready", read: 101, total: 101, understood } }));
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

  it("offers a search by meaning when the words find few", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer());
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [], nextCursor: null, meaning: { status: "reading" } }));
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByText("Siti Hajar");
    fireEvent.change(screen.getByLabelText("Search candidates"), { target: { value: "talent acquisition" } });
    fireEvent.click(await screen.findByRole("button", { name: "Search “talent acquisition” by meaning" }));
    await waitFor(() => { expect(post.mock.calls.some((c) => JSON.stringify(c).includes("\"meaning\":\"talent acquisition\""))).toBe(true); });
    expect(screen.getByRole("button", { name: /Meaning/ }).getAttribute("aria-pressed")).toBe("true");
  });
});
