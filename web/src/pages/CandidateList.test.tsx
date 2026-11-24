import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  localStorage.removeItem("hiringplug.candidates.openAs");
  vi.restoreAllMocks();
});

const summary = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Siti Aisyah", email: null, phone: null, currentTitle: "Accountant",
  currentCompany: null, location: null, workAuthorizationStatus: null, tags: ["relocate"], updatedAt: "2026-11-01T00:00:00Z",
  availability: null, availabilitySince: null, noticePeriod: null, currentSalary: null, expectedSalary: null, salaryRestricted: false,
  workAuthorizationBasis: null, nationality: null, residenceCountry: null, source: null, industry: null,
};

/** The list's requests: the candidates, the values in use, the person's columns (default unless given) and the settings. */
function answer(items: unknown[], columns: string[] | null | (() => string[] | null) = null) {
  return ((path: string) => {
    if (path === "/v1/candidates") return Promise.resolve(ok({ items, nextCursor: null }));
    if (path === "/v1/me/preferences") return Promise.resolve(ok({ candidateColumns: typeof columns === "function" ? columns() : columns }));
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    return Promise.resolve(ok({ tags: ["relocate"], skills: ["SAP"], industries: [], specializations: [], certifications: [] }));
  }) as never;
}

describe("candidate list", () => {
  it("chooses candidates by clicking their rows (one, or all shown) to email each separately", async () => {
    const wei = { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Wei Tan", email: "wei@example.com" };
    vi.spyOn(api, "GET").mockImplementation(answer([summary, wei]));
    renderWithClient(<CandidateList canEmail />);
    expect(screen.queryByRole("checkbox", { name: /Choose/ })).toBeNull(); // no tick boxes
    const row = (await screen.findByText("Wei Tan")).closest("tr") as HTMLElement;
    fireEvent.click(row.querySelector("td:last-child") as Element);
    expect(screen.getByText("1 chosen")).toBeTruthy();
    expect(row.className).toContain("chosen");
    expect(row.textContent).toContain("(chosen)"); // said to screen readers
    fireEvent.click(screen.getByRole("link", { name: "Siti Aisyah" })); // the name opens the candidate, not chooses
    expect(screen.getByText("1 chosen")).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("link", { name: "Wei Tan" }), { key: " " }); // Space on the name unchooses
    expect(screen.queryByText("1 chosen")).toBeNull();
    // Selecting text to copy it does not choose the row.
    const cell = row.querySelector("td:nth-child(2)") as HTMLElement;
    const range = document.createRange();
    range.selectNodeContents(cell);
    window.getSelection()?.removeAllRanges();
    window.getSelection()?.addRange(range);
    fireEvent.click(cell);
    expect(screen.queryByText("1 chosen")).toBeNull();
    window.getSelection()?.removeAllRanges();
    expect(screen.queryByRole("button", { name: /Select all/ }), "nothing chosen: no bar").toBeNull();
    fireEvent.click(row);
    fireEvent.click(screen.getByRole("button", { name: "Select all 2" }));
    expect(screen.getByText("2 chosen")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Select all 2" }), "all are chosen").toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "✕ Clear" }));
    expect(screen.queryByText("2 chosen")).toBeNull();
    fireEvent.click(row);
    fireEvent.click(screen.getByRole("button", { name: "Select all 2" }));
    fireEvent.click(screen.getByRole("button", { name: "✉ Email each separately" }));
    const panel = await screen.findByRole("region", { name: "Email each separately" });
    expect(panel.querySelectorAll(".person-chip")).toHaveLength(2);
    expect(panel.textContent).toContain("Siti Aisyah · no email");
  });

  it("does not choose rows for those who may not send email", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList />);
    await screen.findByText("Siti Aisyah");
    expect(screen.queryByText(/chosen/)).toBeNull();
    fireEvent.click((screen.getByText("Siti Aisyah").closest("tr") as HTMLElement).querySelector("td:last-child") as Element);
    expect(screen.queryByText("1 chosen")).toBeNull();
  });

  it("keeps Filters and AI inside the search bar, Mine only under Filters, and the view tools together", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList canAiSearch canEmail />);
    await screen.findByText("Siti Aisyah");
    const bar = screen.getByLabelText("Search candidates").closest(".bool-box");
    expect(screen.getByRole("link", { name: "✦ AI" }).closest(".bool-box")).toBe(bar);
    const filters = screen.getByRole("button", { name: "Filters" });
    expect(filters.closest(".bool-box")).toBe(bar);
    expect(filters.textContent, "just the funnel").toBe("");
    fireEvent.click(filters);
    fireEvent.click(screen.getByLabelText("Mine only"));
    expect(screen.getByRole("button", { name: "Filters, 1 on" }).textContent).toBe("1");
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove Mine only" }));
    expect(screen.getByRole("button", { name: "Filters" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Candidates" })?.className, "no title on show").toContain("visually-hidden");
    const tools = screen.getByRole("button", { name: "Columns" }).closest(".list-tools");
    expect(screen.getByRole("group", { name: "Clicking a candidate opens" }).closest(".list-tools")).toBe(tools);
  });

  it("adds candidates from CVs dropped on the page", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList canImport />);
    await screen.findByText("Siti Aisyah");
    expect(screen.queryByRole("region", { name: "CV import" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Add candidate|Import CVs/ }), "CVs are dropped instead").toBeNull();
    expect(screen.getByText(/Drop CVs anywhere to upload/).textContent, "no choose-files link").not.toContain("choose files");
    const cv = new File(["%PDF-1.7"], "nurul-cv.pdf", { type: "application/pdf" });
    // Dropped anywhere on the page, here on a row of the list.
    const anywhere = screen.getByText("Siti Aisyah");
    fireEvent.dragEnter(anywhere, { dataTransfer: { types: ["Files"], files: [cv] } });
    fireEvent.drop(anywhere, { dataTransfer: { types: ["Files"], files: [cv] } });
    const panel = await screen.findByRole("region", { name: "CV import" });
    expect(panel.textContent).toContain("nurul-cv.pdf");
    const source = screen.getByLabelText<HTMLSelectElement>("Preset the source");
    expect(source.value, "dropped without seeing the source: it must be picked").toBe("");
    const post = vi.spyOn(api, "POST");
    fireEvent.click(screen.getByRole("button", { name: "Import 1 file" }));
    expect(source.getAttribute("aria-invalid")).toBe("true");
    expect(post).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("region", { name: "CV import" })).toBeNull();
  });

  it("has no CV drop box for those who may not import", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList />);
    await screen.findByText("Siti Aisyah");
    expect(screen.queryByText(/Drop CVs anywhere/)).toBeNull();
  });

  it("shows tags by the name", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList />);
    expect(await screen.findByText("Siti Aisyah")).toBeTruthy();
    expect(screen.getAllByText("relocate").some((el) => el.classList.contains("chip"))).toBe(true);
  });

  it("searches with a boolean and dropdowns, marks where the CV matches, and opens the CV with the words marked", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({
      items: [{ candidate: { ...summary, fullName: "Aina" }, matchedIn: ["cv"], cvSnippet: "Led [[SAP]] [[BW]] upgrades" }],
      nextCursor: null, terms: ["sap", "bw"],
    }));
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByText("Siti Aisyah");
    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    expect(screen.queryByPlaceholderText(/SAP AND/)).toBeNull(); // the box above reads booleans: the filters keep only the tips
    expect(screen.getByText(/any ending/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /^IT skills/ }));
    fireEvent.click(await screen.findByRole("checkbox", { name: "SAP" }));
    fireEvent.click(screen.getByRole("button", { name: /^Status/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Looking out" }));
    fireEvent.change(screen.getByLabelText(/Expects at most/), { target: { value: "12,000" } });
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.change(screen.getByLabelText("Search candidates"), { target: { value: "SAP AND (BW OR BPC) NOT internship" } });

    await waitFor(() => { expect(post.mock.calls.some((c) => JSON.stringify(c).includes("internship"))).toBe(true); });
    post.mock.calls.splice(0, post.mock.calls.findIndex((c) => JSON.stringify(c).includes("internship")));
    expect(sentBody(post)).toEqual({
      limit: 25, skillsMatch: "all", boolean: "SAP AND (BW OR BPC) NOT internship", skills: ["SAP"], availability: ["looking"],
      expectedSalaryMax: { amount: "12000", currency: "MYR" }, includeSalary: true,
    });
    const link = await screen.findByRole("link", { name: "Aina" });
    expect(link.getAttribute("href")).toBe("#/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7?hl=" + encodeURIComponent("SAP AND (BW OR BPC) NOT internship"));
    expect(screen.getAllByText(/^(SAP|BW)$/).filter((el) => el.tagName === "MARK")).toHaveLength(2);

    fireEvent.click(screen.getByRole("button", { name: "Remove Status: Looking out" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect((sentBody(post, 1) as { availability?: string[] }).availability).toBeUndefined();
  });

  it("sorts the whole list on the server and pages on in that order", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: { offset?: number } } }) => {
      if (path === "/v1/candidates") {
        const more = !init?.params?.query?.offset;
        return Promise.resolve(ok({ items: [{ ...summary, id: more ? summary.id : "cand_01jbz6w0m0k8q9x2r3s4t5v6w9", fullName: more ? "Siti Aisyah" : "Zara" }], nextCursor: null, nextOffset: more ? 25 : null }));
      }
      return (answer([summary]) as (p: string) => unknown)(path);
    }) as never);
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByText("Siti Aisyah");
    // Click a header to sort by it, like a spreadsheet; click again to flip.
    fireEvent.click(screen.getByRole("button", { name: /^Expected salary/ }));
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { sort: "expected_salary_asc", includeSalary: true } } }); });
    fireEvent.click(screen.getByRole("button", { name: /^Expected salary/ }));
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { sort: "expected_salary_desc", includeSalary: true } } }); });
    expect(screen.getByRole("columnheader", { name: /Expected salary/ }).getAttribute("aria-sort")).toBe("descending");
    fireEvent.click(await screen.findByRole("button", { name: "Show more" }));
    expect(await screen.findByText("Zara")).toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { sort: "expected_salary_desc", offset: 25, includeSalary: true } } });
    fireEvent.click(screen.getByRole("button", { name: /^Name/ }));
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { sort: "name", includeSalary: true } } }); });
    fireEvent.click(screen.getByRole("button", { name: "Back to newest first" }));
    await waitFor(() => { expect(screen.getByRole("columnheader", { name: /^Name/ }).getAttribute("aria-sort")).toBe("none"); });
    cleanup();

    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList />);
    await screen.findByText("Siti Aisyah");
    expect(screen.getByRole("button", { name: /^Name/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /^Expected salary/ })).toBeNull(); // no salary sorting without salary access
  });

  it("narrows by industry, then subindustry, from two dropdowns", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary])); // the test catalogue: Financial Services > FSI – Banking, IT > ERP
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [], nextCursor: null, terms: [] }));
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByText("Siti Aisyah");
    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    fireEvent.click(await screen.findByRole("button", { name: /^Industry/ }));
    expect(screen.queryByRole("checkbox", { name: "FSI – Banking" })).toBeNull(); // main industries only
    fireEvent.click(screen.getByRole("checkbox", { name: "Financial Services" }));
    fireEvent.click(screen.getByRole("button", { name: /^Subindustry/ }));
    expect(screen.queryByRole("checkbox", { name: /ERP/ })).toBeNull(); // narrowed to Financial Services
    fireEvent.click(screen.getByRole("checkbox", { name: "FSI – Banking" }));
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect((sentBody(post) as { industry?: string[] }).industry).toEqual(["industry_bank", "industry_banking"]);
  });

  it("shows each candidate's status and since when, notice period and salaries, or that salary is restricted", async () => {
    const since = new Date(Date.now() - 5 * 86_400_000).toISOString();
    const rows = [
      { ...summary, availability: "not_looking", availabilitySince: since, noticePeriod: { value: 1, unit: "month" },
        currentSalary: { amount: "8500.00", currency: "MYR", period: "month" },
        expectedSalary: { min: "10000.00", max: "12000.00", currency: "MYR", period: "month" } },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Ali", salaryRestricted: true },
    ];
    vi.spyOn(api, "GET").mockImplementation(answer(rows));
    renderWithClient(<CandidateList />);
    const siti = (await screen.findByText("Siti Aisyah")).closest("tr");
    expect(siti?.textContent).toContain("Not looking");
    expect(siti?.textContent).toContain("· 5 days");
    expect(siti?.textContent).toContain("1 month");
    expect(siti?.textContent).toContain("8,500");
    expect(siti?.textContent).toContain("12,000");
    expect(screen.getByText("Ali").closest("tr")?.textContent).toContain("Restricted");
  });

  it("colours each disc by status, with the work authorization in its colour under the name", async () => {
    const rows = [
      { ...summary, workAuthorizationStatus: "local", availability: "looking" },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Ali", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "rp_t", availability: "placed" },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w9", fullName: "Mei", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "permanent_resident", availability: null },
    ];
    vi.spyOn(api, "GET").mockImplementation(answer(rows));
    renderWithClient(<CandidateList />);
    await screen.findByText("Mei");
    const looking = screen.getByRole("img", { name: "Status: Looking out" });
    expect(looking.closest("tr")?.textContent).toContain("Siti Aisyah");
    expect(looking.textContent).toBe("SA"); // their initials
    expect(looking.style.background).toBe("rgb(255, 243, 176)"); // Looking out's yellow, as on their page
    expect(looking.closest("td")?.textContent).toContain("Local"); // the work authorization under the name
    expect(screen.getByRole("img", { name: "Status: Placed" }).closest("tr")?.textContent).toContain("RP-T");
    expect(screen.getByRole("img", { name: "Status: Not set" }).closest("tr")?.textContent).toContain("Mei");
  });

  it("switches clicks between opening the profile and reading the full resume, and remembers it", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList />);
    const resume = await screen.findByRole("button", { name: "Full resume" });
    expect(screen.getByRole("button", { name: "Profile" }).getAttribute("aria-pressed")).toBe("true");
    await screen.findByRole("link", { name: "Siti Aisyah" }); // the usual table
    fireEvent.click(resume);
    expect(resume.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("region", { name: "Resume of Siti Aisyah" })).toBeTruthy();
    expect(document.querySelector("table.candidates-table")).toBeNull();
    cleanup();
    renderWithClient(<CandidateList />); // later, on this computer
    expect(await screen.findByRole("region", { name: "Resume of Siti Aisyah" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Profile" }));
    expect(screen.getByRole("link", { name: "Siti Aisyah" }).closest("table")).toBeTruthy();
    expect(localStorage.getItem("hiringplug.candidates.openAs")).toBe("profile");
  });

  it("in Full resume, starts again at the first result for a new search, and has no column picker", async () => {
    const wei = { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Wei Tan" };
    vi.spyOn(api, "GET").mockImplementation(answer([summary, wei]));
    localStorage.setItem("hiringplug.candidates.openAs", "resume");
    renderWithClient(<CandidateList />);
    fireEvent.click(await screen.findByRole("button", { name: /Wei Tan/ }));
    expect(screen.getByRole("region", { name: "Resume of Wei Tan" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Columns" })).toBeNull();
    fireEvent.change(screen.getByRole("textbox", { name: "Search candidates" }), { target: { value: "a" } });
    expect(await screen.findByRole("region", { name: "Resume of Siti Aisyah" })).toBeTruthy();
  });

  it("by default leaves out a Work authorization column: it shows in colour under each name", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList canSeeSalary />);
    await screen.findByRole("link", { name: "Siti Aisyah" });
    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent.replace(/[↕▲▼]/g, ""));
    expect(headers).toEqual(["Name", "Current role", "Status", "Notice period", "Current salary", "Expected salary", "Email"]);
  });
});
