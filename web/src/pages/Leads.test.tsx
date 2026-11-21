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
});

it("finds jobs from the exact original CV and clears matches when the selected CV changes", async () => {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok({ items:
    path === "/v1/candidates" ? [{ id: "cand_1", fullName: "Aina Rahman", currentTitle: "QA Engineer" }]
      : path === "/v1/candidates/{candidateId}/documents" ? [
        { id: "doc_1", fileName: "Original CV.pdf", source: "original", status: "extracted" },
        { id: "doc_2", fileName: "Earlier CV.pdf", source: "original", status: "extracted" },
        { id: "doc_3", fileName: "Formatted CV.docx", source: "formatted", status: "extracted" },
      ] : [lead],
  }))) as never);
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ role: "QA Engineer", items: [{ ...lead, title: "QA Automation Engineer" }] }));
  renderWithClient(<Leads permissions={[...manage, "candidates:read", "documents:read"]} />);
  fireEvent.click(screen.getByRole("button", { name: "Find jobs from a CV" }));
  const lookup = screen.getByRole("form", { name: "CV job search" });
  fireEvent.change(within(lookup).getByLabelText("Candidate"), { target: { value: "Aina" } });
  fireEvent.click(await within(lookup).findByRole("button", { name: /Aina Rahman/ }));
  await within(lookup).findByRole("option", { name: "Original CV.pdf" });
  expect(within(lookup).queryByRole("option", { name: "Formatted CV.docx" })).toBeNull();
  fireEvent.click(within(lookup).getByRole("button", { name: "Find jobs" }));
  expect(await screen.findByRole("link", { name: "QA Automation Engineer" })).toBeTruthy();
  expect(sentBody(post)).toMatchObject({ candidateId: "cand_1", documentId: "doc_1", countries: ["MY"] });
  fireEvent.change(within(lookup).getByLabelText("CV"), { target: { value: "doc_2" } });
  expect(screen.queryByRole("link", { name: "QA Automation Engineer" })).toBeNull();
});
