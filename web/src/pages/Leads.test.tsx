import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
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
    expect(screen.getByText("Last check: the site answered HTTP 503")).toBeTruthy();
  });

  it("never makes a link of an address that is not a web address", async () => {
    // eslint-disable-next-line no-script-url -- what an old or hostile record may hold
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
});
