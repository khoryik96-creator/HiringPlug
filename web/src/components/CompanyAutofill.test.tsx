import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { CompanyAutofill, shownValue } from "./CompanyAutofill";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const site = "https://contoh.test";
const idle: Schemas["CompanyAutofill"] = { status: "idle", checkedAt: null, error: null, suggestions: [] };
const done: Schemas["CompanyAutofill"] = {
  status: "done", checkedAt: "2026-11-03T02:00:00Z", error: null,
  suggestions: [
    { id: "csug_1", field: "name", value: "Contoh Teknologi Sdn Bhd", current: "Contoh", source: `schema.org data on ${site}` },
    { id: "csug_2", field: "size", value: "medium", current: null, source: `schema.org data on ${site}` },
  ],
};

describe("company auto-fill", () => {
  it("shows nothing without a website", () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(idle));
    renderWithClient(<CompanyAutofill companyId="comp_1" website={null} />);
    expect(screen.queryByRole("region", { name: "From the website" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("starts reading the website, then waits while it is queued", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(idle));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...idle, status: "queued" }));
    renderWithClient(<CompanyAutofill companyId="comp_1" website={site} />);
    await screen.findByText(/^Reads the name/);
    fireEvent.click(screen.getByRole("button", { name: "Fill in from website" }));
    expect(await screen.findByText("Reading the website…")).toBeTruthy();
    expect(post.mock.calls[0]?.[0]).toBe("/v1/companies/{companyId}/autofill");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Fill in from website" }).disabled).toBe(true);
  });

  it("lists suggestions next to what is recorded, and sends the decision", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(done));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...done, suggestions: done.suggestions.slice(1) }));
    renderWithClient(<CompanyAutofill companyId="comp_1" website={site} />);
    const name = await screen.findByRole("listitem", { name: "Name" });
    expect(name.textContent).toContain("Contoh Teknologi Sdn Bhd");
    expect(name.textContent).toContain("Now: Contoh");
    expect(name.textContent).toContain(`From schema.org data on ${site}`);
    expect(screen.getByRole("listitem", { name: "Size" }).textContent).toContain("51–200 people");
    expect(screen.getByRole("button", { name: "Read the website again" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Replace" }));
    await waitFor(() => { expect(screen.queryByRole("listitem", { name: "Name" })).toBeNull(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/companies/{companyId}/autofill/{suggestionId}");
    expect(sentBody(post)).toEqual({ accept: true });
  });

  it("says why reading failed", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...idle, status: "failed", error: "the website's robots.txt asks robots not to read it" }));
    renderWithClient(<CompanyAutofill companyId="comp_1" website={site} />);
    expect((await screen.findByRole("status")).textContent).toContain("robots.txt");
  });

  it("shows sizes and countries in words", () => {
    expect(shownValue("size", "enterprise")).toBe("Over 1,000 people");
    expect(shownValue("country", "MY")).toBe("Malaysia (MY)");
    expect(shownValue("phone", "+60 3-1234 5678")).toBe("+60 3-1234 5678");
  });
});
