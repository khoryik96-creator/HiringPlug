import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CandidateList } from "./CandidateList";
import { CompanyList } from "./CompanyList";
import { ContactList } from "./ContactList";

afterEach(() => { cleanup(); vi.restoreAllMocks(); window.sessionStorage.clear(); });

/** The lists' other reads (settings, preferences, used values), and the list itself. */
function reads(list: unknown) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/settings/fields" ? ok(testFieldSettings())
      : path === "/v1/me/preferences" ? ok({ candidateColumns: null })
        : path === "/v1/candidates" || path === "/v1/companies" || path === "/v1/contacts" ? ok(list)
          : ok({ tags: [], skills: [], industries: [], specializations: [], certifications: [] }),
  )) as never);
}
const queryOf = (get: ReturnType<typeof reads>, path: string) =>
  get.mock.calls.filter((c) => c[0] === path).map((c) => (c[1] as { params: { query: Record<string, unknown> } }).params.query).at(-1);

describe("boolean rules in a list's search box", () => {
  it("runs the candidates' boolean search of CVs and profiles, with the filters", async () => {
    reads({ items: [], nextCursor: null });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [], nextCursor: null, terms: ["sap"] }));
    renderWithClient(<CandidateList />);
    const box = await screen.findByLabelText("Search candidates");
    fireEvent.change(box, { target: { value: "SAP AND (BW OR BPC) NOT intern*" } });
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post, post.mock.calls.length - 1)).toMatchObject({ boolean: "SAP AND (BW OR BPC) NOT intern*" });
    expect((sentBody(post, post.mock.calls.length - 1) as { q?: string }).q, "not also a name search").toBeUndefined();
    expect(screen.getByText("Boolean")).toBeTruthy();
    expect([...document.querySelectorAll(".bool-rule")].map((e) => e.textContent)).toEqual(["AND", "(", "OR", ")", "NOT", "*"]);
  });

  it("keeps a plain search a name search", async () => {
    const get = reads({ items: [], nextCursor: null });
    const post = vi.spyOn(api, "POST");
    renderWithClient(<CandidateList />);
    fireEvent.change(await screen.findByLabelText("Search candidates"), { target: { value: "Tan - Maju" } });
    await waitFor(() => { expect(queryOf(get, "/v1/candidates")).toEqual({ q: "Tan - Maju" }); });
    expect(post).not.toHaveBeenCalled();
  });

  it("sends companies and contacts a boolean, or a plain search", async () => {
    const get = reads({ items: [], nextCursor: null });
    renderWithClient(<CompanyList canCreate={false} />);
    const box = await screen.findByLabelText("Search companies");
    fireEvent.change(box, { target: { value: '(bank OR insurance) NOT "staffing"' } });
    await waitFor(() => { expect(queryOf(get, "/v1/companies")).toMatchObject({ boolean: '(bank OR insurance) NOT "staffing"' }); });
    fireEvent.change(box, { target: { value: "Maju" } });
    await waitFor(() => { expect(queryOf(get, "/v1/companies")).toMatchObject({ q: "Maju" }); });
    cleanup();
    renderWithClient(<ContactList canCreate={false} />);
    fireEvent.change(await screen.findByLabelText("Search contacts"), { target: { value: "HR AND manager" } });
    await waitFor(() => { expect(queryOf(get, "/v1/contacts")).toMatchObject({ boolean: "HR AND manager" }); });
  });
});
