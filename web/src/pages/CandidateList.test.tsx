import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
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
    return Promise.resolve(ok({ tags: ["relocate"], skills: ["SAP"], industries: [], specializations: [] }));
  }) as never;
}

describe("candidate list", () => {
  it("shows tags by the name", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    renderWithClient(<CandidateList canCreate={false} />);
    expect(await screen.findByText("Siti Aisyah")).toBeTruthy();
    expect(screen.getAllByText("relocate").some((el) => el.classList.contains("chip"))).toBe(true);
  });

  it("searches with a boolean and dropdowns, marks where the CV matches, and opens the CV with the words marked", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary]));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({
      items: [{ candidate: { ...summary, fullName: "Aina" }, matchedIn: ["cv"], cvSnippet: "Led [[SAP]] [[BW]] upgrades" }],
      nextCursor: null, terms: ["sap", "bw"],
    }));
    renderWithClient(<CandidateList canCreate canSeeSalary />);
    await screen.findByText("Siti Aisyah");
    fireEvent.click(screen.getByRole("button", { name: "Advanced search" }));
    fireEvent.change(screen.getByPlaceholderText(/SAP AND/), { target: { value: "SAP AND (BW OR BPC) NOT internship" } });
    expect(screen.getByText("internship").className).toBe("bool-not"); // read as left out
    fireEvent.click(screen.getByRole("button", { name: /^Skills/ }));
    fireEvent.click(await screen.findByRole("checkbox", { name: "SAP" }));
    fireEvent.click(screen.getByRole("button", { name: /^Status/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Looking out" }));
    fireEvent.change(screen.getByLabelText(/Expects at most/), { target: { value: "12,000" } });
    fireEvent.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({
      limit: 25, skillsMatch: "all", boolean: "SAP AND (BW OR BPC) NOT internship", skills: ["SAP"], availability: ["looking"],
      expectedSalaryMax: { amount: "12000", currency: "MYR" },
    });
    const link = await screen.findByRole("link", { name: "Aina" });
    expect(link.getAttribute("href")).toBe("#/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7?hl=" + encodeURIComponent("SAP AND (BW OR BPC) NOT internship"));
    expect(screen.getAllByText(/^(SAP|BW)$/).filter((el) => el.tagName === "MARK")).toHaveLength(2);
    expect(screen.queryByPlaceholderText("First name")).toBeNull(); // adding waits while searching

    fireEvent.click(screen.getByRole("button", { name: "Remove Status: Looking out" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect((sentBody(post, 1) as { availability?: string[] }).availability).toBeUndefined();
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
    renderWithClient(<CandidateList canCreate={false} />);
    const siti = (await screen.findByText("Siti Aisyah")).closest("tr");
    expect(siti?.textContent).toContain("Not looking");
    expect(siti?.textContent).toContain("· 5 days");
    expect(siti?.textContent).toContain("1 month");
    expect(siti?.textContent).toContain("8,500");
    expect(siti?.textContent).toContain("12,000");
    expect(screen.getByText("Ali").closest("tr")?.textContent).toContain("Restricted");
  });

  it("marks each name with a bar coloured by work authorization", async () => {
    const rows = [
      { ...summary, workAuthorizationStatus: "local" },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Ali", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "rp_t" },
      { ...summary, id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w9", fullName: "Mei", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "permanent_resident" },
    ];
    vi.spyOn(api, "GET").mockImplementation(answer(rows));
    renderWithClient(<CandidateList canCreate={false} />);
    await screen.findByText("Mei");
    expect(screen.getByRole("img", { name: "Work authorization: Local" }).closest("tr")?.textContent).toContain("Siti Aisyah");
    expect(screen.getByRole("img", { name: "Work authorization: RP-T" }).closest("tr")?.textContent).toContain("Ali");
    expect(screen.getByRole("img", { name: "Work authorization: PR" }).closest("tr")?.textContent).toContain("Mei");
  });

  it("shows the columns the person chose, and saves a new choice for them", async () => {
    const row = { ...summary, nationality: "SG", residenceCountry: "MY", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "spouse_visa" };
    let saved = ["nationality", "residence", "workAuthorization"];
    vi.spyOn(api, "GET").mockImplementation(answer([row], () => saved));
    const put = vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { candidateColumns: string[] } }) => {
      saved = init.body.candidateColumns;
      return Promise.resolve(ok({ candidateColumns: saved }));
    }) as never);
    renderWithClient(<CandidateList canCreate={false} />);
    await screen.findByRole("columnheader", { name: "Nationality" });
    expect(screen.getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["Name", "Nationality", "Residence", "Work authorization"]);
    const siti = screen.getByText("Siti Aisyah").closest("tr");
    expect(siti?.textContent).toContain("Singapore");
    expect(siti?.textContent).toContain("Malaysia");
    expect(siti?.textContent).toContain("Spouse visa");

    fireEvent.click(screen.getByRole("button", { name: "Columns" }));
    fireEvent.click(screen.getByLabelText("Industry"));
    await waitFor(() => {
      expect(put).toHaveBeenCalledWith("/v1/me/preferences", { body: { candidateColumns: ["nationality", "residence", "workAuthorization", "industry"] } });
    });
    expect(await screen.findByRole("columnheader", { name: "Industry" })).toBeTruthy();
  });

  it("saves quick changes of columns one after another, so the last one is kept", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary], ["nationality"]));
    const pending: ((v: unknown) => void)[] = [];
    const put = vi.spyOn(api, "PUT").mockImplementation((() => new Promise((resolve) => { pending.push(resolve); })) as never);
    renderWithClient(<CandidateList canCreate={false} />);
    await screen.findByRole("columnheader", { name: "Nationality" });
    fireEvent.click(screen.getByRole("button", { name: "Columns" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Residence" }));
    fireEvent.click(await screen.findByRole("checkbox", { name: "Industry" }));
    expect(await screen.findByRole("columnheader", { name: "Industry" })).toBeTruthy(); // shown at once
    await new Promise((r) => { setTimeout(r, 50); });
    expect(put).toHaveBeenCalledTimes(1); // the second waits for the first
    pending[0]?.(ok({ candidateColumns: ["nationality", "residence"] }));
    await waitFor(() => { expect(put).toHaveBeenCalledTimes(2); });
    expect(put.mock.calls[1]).toEqual(["/v1/me/preferences", { body: { candidateColumns: ["nationality", "residence", "industry"] } }]);
  });
});
