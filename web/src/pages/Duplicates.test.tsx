import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { Duplicates } from "./Duplicates";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("lists groups of possible duplicate companies with why", async () => {
  const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ reasons: ["same_website", "similar_name"], companies: [
    { id: "comp_2", name: "ACME Berhad", city: "Penang", website: "acme.example", industry: null, reasons: [] },
    { id: "comp_1", name: "Acme Sdn Bhd", city: null, website: null, industry: null, reasons: [] },
  ] }] }));
  renderWithClient(<Duplicates kind="companies" />);
  expect(await screen.findByText("same website, similar name")).toBeTruthy();
  expect(screen.getByRole("link", { name: "ACME Berhad" }).getAttribute("href")).toBe("#/companies/comp_2");
  expect(get.mock.calls[0]?.[0]).toBe("/v1/companies/duplicates");
});

it("says when there are none", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
  renderWithClient(<Duplicates kind="contacts" />);
  expect(await screen.findByText("No possible duplicates among your contacts.")).toBeTruthy();
});
