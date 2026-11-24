import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import * as companion from "../companion";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Leads } from "./Leads";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const lead: Schemas["JobLead"] = {
  id: "lead_1", version: 1, sourceId: "lsrc_1", sourceKind: "careers_page", companyName: "Contoh Berhad", companyId: null,
  title: "Account Executive", location: "Kuala Lumpur, MY", url: "https://contoh.com.my/careers", postedAt: "2026-10-05T00:00:00Z",
  firstSeenAt: "2026-11-01T00:00:00Z", lastSeenAt: "2026-11-03T00:00:00Z", closedAt: null, status: "new", notes: null,
};
const source: Schemas["LeadSource"] = {
  id: "lsrc_1", version: 1, kind: "careers_page", identifier: "https://contoh.com.my/careers", name: "Contoh Berhad", companyId: null,
  active: true, openLeads: 1, lastCheckedAt: "2026-11-03T00:00:00Z", lastError: "the site answered HTTP 503", nextCheckAt: "2099-01-01T00:00:00Z", createdAt: "2026-11-01T00:00:00Z",
};
const reads = () =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok({ items: path === "/v1/lead-sources" ? [source] : [lead] }))) as never);

const manage = ["leads:read", "leads:manage"] as const;

describe("leads", () => {
  it("lists leads and sources, with why a check failed", async () => {
    reads();
    renderWithClient(<Leads permissions={[...manage]} />);
    expect((await screen.findByRole("link", { name: "Account Executive" })).getAttribute("href")).toBe("https://contoh.com.my/careers");
    expect(screen.queryByRole("region", { name: "Sources" }), "the sources fold away behind their button").toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Sources" }));
    expect(await screen.findByText("Last check: the site answered HTTP 503")).toBeTruthy();
  });

  it("never makes a link of an address that is not a web address", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok({ items: path === "/v1/lead-sources" ? [source] : [{ ...lead, url: "javascript:alert(1)" }] }))) as never);
    renderWithClient(<Leads permissions={[...manage]} />);
    expect(await screen.findByText("Account Executive")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Account Executive" })).toBeNull();
  });

  it("changes a lead's status", async () => {
    reads();
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, status: "contacted" }));
    renderWithClient(<Leads permissions={[...manage]} />);
    fireEvent.change(await screen.findByLabelText("Status of Account Executive at Contoh Berhad"), { target: { value: "contacted" } });
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 1, status: "contacted" });
  });

  it("adds a job board source", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(source));
    renderWithClient(<Leads permissions={[...manage]} />);
    fireEvent.click(screen.getByRole("button", { name: "Sources" }));
    const form = await screen.findByRole("form", { name: "New source" });
    fireEvent.change(within(form).getByLabelText("Board"), { target: { value: "acme" } });
    fireEvent.change(within(form).getByLabelText("Company name"), { target: { value: "Acme" } });
    fireEvent.click(within(form).getByRole("button", { name: "Add source" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ kind: "greenhouse", identifier: "acme", name: "Acme" });
  });

  it("reading only: no changes offered", async () => {
    reads();
    renderWithClient(<Leads permissions={["leads:read"]} />);
    expect(await screen.findByText("Account Executive")).toBeTruthy();
    expect(screen.queryByRole("form", { name: "New source" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Check now" })).toBeNull();
  });

  it("saves an ad found elsewhere from the + Save a lead button", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(lead));
    renderWithClient(<Leads permissions={[...manage]} />);
    expect(screen.queryByRole("form", { name: "Save an ad" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "+ Save a lead" }));
    const form = screen.getByRole("form", { name: "Save an ad" });
    fireEvent.change(within(form).getByLabelText("Company"), { target: { value: "Contoh Berhad" } });
    fireEvent.change(within(form).getByLabelText("Job"), { target: { value: "Account Executive" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save lead" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    await waitFor(() => { expect(screen.queryByRole("form", { name: "Save an ad" })).toBeNull(); });
    cleanup();
    reads();
    renderWithClient(<Leads permissions={["leads:read"]} />);
    expect(screen.queryByRole("button", { name: "+ Save a lead" })).toBeNull();
  });

  it("opens Find HR for a lead in a panel beside the list, one lead at a time", async () => {
    const other = { ...lead, id: "lead_2", companyName: "Nusantara Logistik Sdn Bhd", title: "HR Executive" };
    vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: false, version: null });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok({ items: path === "/v1/job-leads" ? [lead, other] : [] }))) as never);
    renderWithClient(<Leads permissions={[...manage, "companies:read", "companies:create", "contacts:read"]} />);
    const find = await screen.findByRole("button", { name: "Find HR for Contoh Berhad" });
    expect(find.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(find);
    const panel = screen.getByRole("complementary", { name: "Find HR for Contoh Berhad" });
    expect(within(panel).getByRole("heading", { name: "1 · Which company is it?" })).toBeTruthy();
    expect(find.getAttribute("aria-pressed")).toBe("true");
    expect(find.closest("tr")?.className).toBe("lead-open");
    // Another lead's button switches the panel to it.
    fireEvent.click(screen.getByRole("button", { name: "Find HR for Nusantara Logistik Sdn Bhd" }));
    expect(screen.getByRole("complementary", { name: "Find HR for Nusantara Logistik Sdn Bhd" })).toBeTruthy();
    expect(screen.queryByRole("complementary", { name: "Find HR for Contoh Berhad" })).toBeNull();
    expect(find.getAttribute("aria-pressed")).toBe("false");
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(screen.getByRole("region", { name: "Hiring leads" })).toBeTruthy();
  });

  it("offers Find HR only to those who may read companies and contacts", async () => {
    reads();
    renderWithClient(<Leads permissions={[...manage, "companies:read"]} />);
    expect(await screen.findByText("Account Executive")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Find HR/ })).toBeNull();
    cleanup();
    reads();
    renderWithClient(<Leads permissions={[...manage, "contacts:read"]} />);
    expect(await screen.findByText("Account Executive")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Find HR/ })).toBeNull();
  });
});
