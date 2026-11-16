import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { drawHeadings, ok, renderWithClient, testFieldSettings } from "../test-utils";
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

/** The list's requests: the candidates, the values in use, the person's columns and the settings. */
function answer(items: unknown[], columns: string[] | null | (() => string[] | null) = null) {
  return ((path: string) => {
    if (path === "/v1/candidates") return Promise.resolve(ok({ items, nextCursor: null }));
    if (path === "/v1/me/preferences") return Promise.resolve(ok({ candidateColumns: typeof columns === "function" ? columns() : columns }));
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    return Promise.resolve(ok({ tags: ["relocate"], skills: ["SAP"], industries: [], specializations: [], certifications: [] }));
  }) as never;
}

describe("candidate list columns", () => {
  it("shows the columns the person chose, and saves a new choice for them", async () => {
    const row = { ...summary, nationality: "SG", residenceCountry: "MY", workAuthorizationStatus: "expat_eligible", workAuthorizationBasis: "spouse_visa" };
    let saved = ["nationality", "residence", "workAuthorization"];
    vi.spyOn(api, "GET").mockImplementation(answer([row], () => saved));
    const put = vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { candidateColumns: string[] } }) => {
      saved = init.body.candidateColumns;
      return Promise.resolve(ok({ candidateColumns: saved }));
    }) as never);
    renderWithClient(<CandidateList />);
    await screen.findByRole("columnheader", { name: /^Nationality/ });
    expect(screen.getAllByRole("columnheader").map((h) => h.textContent.replace(/[↕▲▼]/g, ""))).toEqual(["Name", "Nationality", "Residence", "Work authorization"]);
    const siti = screen.getByText("Siti Aisyah").closest("tr");
    expect(siti?.textContent).toContain("Singapore");
    expect(siti?.textContent).toContain("Malaysia");
    expect(siti?.textContent).toContain("Spouse visa");

    fireEvent.click(screen.getByRole("button", { name: "Columns ▾" }));
    fireEvent.click(screen.getByLabelText("Industry"));
    await waitFor(() => {
      expect(put).toHaveBeenCalledWith("/v1/me/preferences", { body: { candidateColumns: ["nationality", "residence", "workAuthorization", "industry"] } });
    });
    expect(await screen.findByRole("columnheader", { name: /^Industry/ })).toBeTruthy();
  });

  it("moves a column by dragging its heading, and saves the new order", async () => {
    let saved = ["nationality", "residence", "workAuthorization"];
    vi.spyOn(api, "GET").mockImplementation(answer([summary], () => saved));
    const put = vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { candidateColumns: string[] } }) => {
      saved = init.body.candidateColumns;
      return Promise.resolve(ok({ candidateColumns: saved }));
    }) as never);
    drawHeadings();
    renderWithClient(<CandidateList />);
    const wa = await screen.findByRole("columnheader", { name: "Work authorization" });
    fireEvent.pointerDown(wa, { button: 0, clientX: 350 });
    fireEvent.pointerMove(document, { clientX: 120 }); // over the left half of Nationality
    fireEvent.pointerUp(document, { clientX: 120 });
    await waitFor(() => {
      expect(put).toHaveBeenCalledWith("/v1/me/preferences", { body: { candidateColumns: ["workAuthorization", "nationality", "residence"] } });
    });
    await waitFor(() => {
      expect(screen.getAllByRole("columnheader").map((h) => h.getAttribute("aria-label"))).toEqual(["Name", "Work authorization", "Nationality", "Residence"]);
    });
    fireEvent.click(screen.getByRole("button", { name: "Columns ▾" }));
    expect(screen.queryByRole("button", { name: /^Move / }), "no arrows: headings are dragged").toBeNull();
  });

  it("saves quick changes of columns one after another, so the last one is kept", async () => {
    vi.spyOn(api, "GET").mockImplementation(answer([summary], ["nationality"]));
    const pending: ((v: unknown) => void)[] = [];
    const put = vi.spyOn(api, "PUT").mockImplementation((() => new Promise((resolve) => { pending.push(resolve); })) as never);
    renderWithClient(<CandidateList />);
    await screen.findByRole("columnheader", { name: /^Nationality/ });
    fireEvent.click(screen.getByRole("button", { name: "Columns ▾" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Residence" }));
    fireEvent.click(await screen.findByRole("checkbox", { name: "Industry" }));
    expect(await screen.findByRole("columnheader", { name: /^Industry/ })).toBeTruthy(); // shown at once
    await new Promise((r) => { setTimeout(r, 50); });
    expect(put).toHaveBeenCalledTimes(1); // the second waits for the first
    pending[0]?.(ok({ candidateColumns: ["nationality", "residence"] }));
    await waitFor(() => { expect(put).toHaveBeenCalledTimes(2); });
    expect(put.mock.calls[1]).toEqual(["/v1/me/preferences", { body: { candidateColumns: ["nationality", "residence", "industry"] } }]);
  });

});
