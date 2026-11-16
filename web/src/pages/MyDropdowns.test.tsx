import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { MyDropdowns } from "./MyDropdowns";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

/** What the latest save sent. */
const last = (put: { mock: { calls: unknown[][] } }) => sentBody(put, put.mock.calls.length - 1);

/** A server that keeps what is saved; returns the PUT spy. */
function server(favourites: object) {
  // As the server does: the kinds sent replace those saved; the others stay.
  let saved = { favourites };
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok(saved))) as never);
  return vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { favourites: object } }) => {
    saved = { favourites: { ...saved.favourites, ...init.body.favourites } };
    return Promise.resolve(ok(saved));
  }) as never);
}

describe("My dropdowns", () => {
  it("keeps the person's countries on top, in the order they chose", async () => {
    const put = server({ country: ["MY"] });
    renderWithClient(<MyDropdowns />);
    const countries = await screen.findByRole("region", { name: /Countries/ });
    expect(within(countries).getByText("Malaysia")).toBeTruthy();
    const add = within(countries).getByLabelText<HTMLSelectElement>("Add a country");
    expect([...add.options].some((o) => o.text.startsWith("Malaysia")), "one chosen is not offered again").toBe(false);
    expect([...add.options].some((o) => o.text.startsWith("Germany")), "every country can be chosen").toBe(true);
    fireEvent.change(add, { target: { value: "SG" } });
    await waitFor(() => { expect(last(put)).toEqual({ favourites: { country: ["MY", "SG"] } }); });
    fireEvent.click(await within(countries).findByRole("button", { name: "Move Singapore up" }));
    await waitFor(() => { expect(last(put)).toEqual({ favourites: { country: ["SG", "MY"] } }); });
    fireEvent.click(within(countries).getByRole("button", { name: "Remove Malaysia" }));
    await waitFor(() => { expect(last(put)).toEqual({ favourites: { country: ["SG"] } }); });
  });

  it("switches a list's words on and off, and clears them, without touching the list", async () => {
    const put = server({ itSkill: ["Python", "Golang"], certification: ["ACCA"] });
    renderWithClient(<MyDropdowns />);
    const skills = await screen.findByRole("region", { name: "IT skills" });
    await waitFor(() => { expect(within(skills).getAllByRole("switch").map((s) => s.textContent)).toEqual(["Java", "Python", "SAP", "Golang (no longer in the list)"]); });
    expect(within(skills).getByRole("switch", { name: "Python" }).getAttribute("aria-checked")).toBe("true");
    expect(within(skills).getByText("1 favourite on top"), "Golang is no longer in the list").toBeTruthy();
    const gone = within(skills).getByRole("switch", { name: "Golang (no longer in the list)" });
    fireEvent.click(gone);
    await waitFor(() => { expect(last(put), "it can still be switched off").toEqual({ favourites: { itSkill: ["Python"] } }); });
    fireEvent.click(within(skills).getByRole("switch", { name: "Java" }));
    await waitFor(() => { expect(last(put), "only the kind changed").toEqual({ favourites: { itSkill: ["Python", "Java"] } }); });
    await waitFor(() => { expect(within(skills).getByRole("switch", { name: "Java" }).getAttribute("aria-checked")).toBe("true"); });
    fireEvent.click(within(screen.getByRole("region", { name: "Professional certifications" })).getByRole("button", { name: "Clear" }));
    await waitFor(() => { expect(last(put)).toEqual({ favourites: { certification: [] } }); });
  });

  it("stars an industry or one of its subindustries, found by a search", async () => {
    const put = server({});
    renderWithClient(<MyDropdowns />);
    const section = await screen.findByRole("region", { name: "Industries" });
    fireEvent.change(within(section).getByLabelText("Find an industry"), { target: { value: "banking" } });
    expect(within(section).queryByText("Education"), "the search narrows the industries").toBeNull();
    fireEvent.click(within(section).getByRole("switch", { name: "FSI – Banking" }));
    await waitFor(() => { expect(last(put)).toEqual({ favourites: { industry: ["industry_banking"] } }); });
  });
});
