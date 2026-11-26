import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type CompanySummary } from "../api/client";
import { drawHeadings, ok, renderWithClient, testFieldSettings } from "../test-utils";
import { CompanyList } from "./CompanyList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

const company = (id: string, name: string, country: string | null = null): CompanySummary => ({
  id, name, type: "client", category: null, industry: null, city: null, country, website: null, linkedinUrl: null, updatedAt: "2026-11-01T00:00:00Z",
});

it("pages through every company, 100 at a time", async () => {
  const asked: Record<string, unknown>[] = [];
  vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: Record<string, unknown> } }) => {
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    const query = init?.params?.query ?? {};
    asked.push(query);
    return Promise.resolve(ok(query.cursor
      ? { items: [company("comp_01jbz6w0m0k8q9x2r3s4t5v6b1", "Maju Holdings")], nextCursor: null }
      : { items: [company("comp_01jbz6w0m0k8q9x2r3s4t5v6a1", "Acme Foods")], nextCursor: "comp_01jbz6w0m0k8q9x2r3s4t5v6a1" }));
  }) as never);
  renderWithClient(<CompanyList canCreate={false} />);
  expect(await screen.findByRole("link", { name: "Acme Foods" })).toBeTruthy();
  expect(asked[0]).toEqual({ limit: 100 });
  fireEvent.click(screen.getByRole("button", { name: "Show more" }));
  expect(await screen.findByRole("link", { name: "Maju Holdings" })).toBeTruthy();
  await waitFor(() => { expect(asked.at(-1)).toEqual({ limit: 100, cursor: "comp_01jbz6w0m0k8q9x2r3s4t5v6a1" }); });
  expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
});

it("moves a column, cells and all, by dragging its heading", async () => {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/settings/fields" ? testFieldSettings()
    : { items: [{ ...company("comp_01jbz6w0m0k8q9x2r3s4t5v6a1", "Acme Foods"), city: "Penang", industry: "Food" }], nextCursor: null }))) as never);
  drawHeadings();
  renderWithClient(<CompanyList canCreate={false} />);
  const row = (await screen.findByRole("link", { name: "Acme Foods" })).closest("tr");
  const names = () => screen.getAllByRole("columnheader").map((h) => h.getAttribute("aria-label"));
  expect(names()).toEqual(["Name", "Category", "Type", "Industry", "City", "Website", "LinkedIn"]);
  fireEvent.pointerDown(screen.getByRole("columnheader", { name: "City" }), { button: 0, clientX: 450 });
  fireEvent.pointerMove(document, { clientX: 110 }); // over the left half of Category
  fireEvent.pointerUp(document, { clientX: 110 });
  fireEvent.click(screen.getByRole("columnheader", { name: "City" })); // the browser's click as the press ends
  expect(names()).toEqual(["Name", "City", "Category", "Type", "Industry", "Website", "LinkedIn"]);
  expect(row?.children[1]?.textContent, "the cells follow").toBe("Penang");
  fireEvent.click(screen.getByRole("button", { name: "Reset column order" }));
  expect(names()).toEqual(["Name", "Category", "Type", "Industry", "City", "Website", "LinkedIn"]);
});
