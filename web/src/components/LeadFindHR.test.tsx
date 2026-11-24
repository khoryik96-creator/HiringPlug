import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company, type Permission, type Schemas } from "../api/client";
import * as companion from "../companion";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { LeadFindHR } from "./LeadFindHR";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const lead: Schemas["JobLead"] = {
  id: "lead_1", version: 4, sourceId: null, sourceKind: null, companyName: "Contoh Logistik Sdn Bhd", companyId: null,
  title: "Warehouse Supervisor", location: "Shah Alam, MY", url: null, postedAt: null,
  firstSeenAt: "2026-11-01T00:00:00Z", lastSeenAt: "2026-11-03T00:00:00Z", closedAt: null, status: "new", notes: null,
};
const company = {
  id: "comp_1", version: 2, name: "Contoh Logistik Sdn Bhd", city: "Shah Alam", linkedinUrl: "https://www.linkedin.com/company/contoh-logistik",
} as Company;
const named: Schemas["CompanyNamedLike"][] = [
  { id: "comp_1", name: "Contoh Logistik Sdn Bhd", city: "Shah Alam", website: null, linkedinUrl: company.linkedinUrl, industry: null, sameName: true },
  { id: "comp_2", name: "Contoh Logistics Berhad", city: null, website: null, linkedinUrl: null, industry: null, sameName: false },
];
const ALL: Permission[] = ["leads:read", "leads:manage", "companies:read", "companies:create", "companies:update", "contacts:read", "contacts:create"];

function serve({ matches = named, saved = company, version = "1.6.0" }: { matches?: Schemas["CompanyNamedLike"][]; saved?: Company; version?: string } = {}) {
  vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: true, version });
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/companies/named-like" ? { items: matches }
      : path === "/v1/companies/{companyId}/hr-scan" ? { scan: null }
        : path === "/v1/companies/{companyId}" ? saved : { items: [] },
  ))) as never);
}

const show = (over: Partial<Schemas["JobLead"]> = {}, permissions = ALL, onClose = vi.fn()) =>
  renderWithClient(<LeadFindHR lead={{ ...lead, ...over }} permissions={permissions} onClose={onClose} />);

describe("a lead's Find HR panel", () => {
  it("lists the companies with the same or a similar name, and links the lead to the one chosen", async () => {
    const get = serve();
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, version: 5, companyId: "comp_1" }));
    show();
    expect(screen.getByRole("heading", { name: "Find HR · Contoh Logistik Sdn Bhd" })).toBeTruthy();
    const same = (await screen.findByText("Contoh Logistics Berhad")).closest("li") as HTMLElement;
    expect(same.textContent).toContain("in HiringPlug · similar name");
    expect(screen.getByText("in HiringPlug · same name · Shah Alam")).toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/companies/named-like", { params: { query: { name: "Contoh Logistik Sdn Bhd" } } });
    // A company with the same name: adding a new one waits behind a link.
    expect(screen.queryByRole("form", { name: "New company" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "This one: Contoh Logistik Sdn Bhd" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(patch.mock.calls[0]?.[0]).toBe("/v1/job-leads/{jobLeadId}");
    expect(sentBody(patch)).toEqual({ version: 4, companyId: "comp_1" });
    // Step 2: the company's Find HR, in the panel.
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Find HR people" })).toBeTruthy();
  });

  it("adds the company with its LinkedIn page, then links the lead", async () => {
    serve({ matches: [] });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...company, id: "comp_9", name: "Contoh Logistik" }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, companyId: "comp_9" }));
    show();
    const form = await screen.findByRole("form", { name: "New company" });
    expect(screen.getByText("No company in HiringPlug has this name or one like it.")).toBeTruthy();
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Contoh Logistik" } });
    expect(within(form).getByRole("heading", { name: "Add Contoh Logistik as a new company" })).toBeTruthy();
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "linkedin.com/company/contoh-logistik" } });
    fireEvent.click(within(form).getByRole("button", { name: "Add and link" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/companies");
    expect(sentBody(post)).toEqual({ name: "Contoh Logistik", linkedinUrl: "https://www.linkedin.com/company/contoh-logistik/" });
    expect(sentBody(patch)).toEqual({ version: 4, companyId: "comp_9" });
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
  });

  it("offers to link the company that has the name already", async () => {
    serve({ matches: [] });
    vi.spyOn(api, "POST").mockResolvedValue(failure("COMPANY_DUPLICATE_NAME", "A company with this name exists.", { existingCompanyId: "comp_1" }, 409));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, companyId: "comp_1" }));
    show();
    fireEvent.click(await screen.findByRole("button", { name: "Add and link" }));
    fireEvent.click(await screen.findByRole("button", { name: "Link the lead to that one" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 4, companyId: "comp_1" });
  });

  it("finds the company's LinkedIn page from its name with the companion", async () => {
    serve({ matches: [] });
    const call = vi.spyOn(companion, "companionCall").mockResolvedValue({ companies: [
      { url: "https://www.linkedin.com/company/contoh-logistik/", name: "Contoh Logistik", subtitle: "Truck Transportation • Shah Alam, Selangor" },
      { url: "https://www.linkedin.com/company/contoh-group/", name: "Contoh Group", subtitle: "" },
    ] });
    show();
    const form = await screen.findByRole("form", { name: "New company" });
    fireEvent.click(within(form).getByRole("button", { name: "Find on LinkedIn" }));
    const found = await screen.findByRole("list", { name: "Found on LinkedIn" });
    expect(call.mock.calls[0]?.[0]).toBe("companySearch");
    expect(call.mock.calls[0]?.[1]).toMatchObject({ name: "Contoh Logistik Sdn Bhd" });
    expect(found.textContent).toContain("Truck Transportation • Shah Alam, Selangor");
    fireEvent.click(within(found).getByRole("button", { name: /Contoh Logistik/ }));
    expect(within(form).getByLabelText<HTMLInputElement>("LinkedIn page").value).toBe("https://www.linkedin.com/company/contoh-logistik/");
    expect(within(found).getByRole("button", { name: /Contoh Logistik/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("asks for the page to be pasted when the companion is too old to find it", async () => {
    serve({ matches: [], version: "1.5.0" });
    show();
    const form = await screen.findByRole("form", { name: "New company" });
    expect(within(form).queryByRole("button", { name: "Find on LinkedIn" })).toBeNull();
    expect(within(form).getByText(/update the LinkedIn companion to 1.6.0/)).toBeTruthy();
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "https://example.com/contoh" } });
    expect(within(form).getByText(/not a LinkedIn company page/)).toBeTruthy();
    expect(within(form).getByRole<HTMLButtonElement>("button", { name: "Add and link" }).disabled).toBe(true);
  });

  it("shows a linked lead's Find HR at once", async () => {
    const get = serve();
    show({ companyId: "comp_1" });
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Contoh Logistik Sdn Bhd ↗" }).getAttribute("target")).toBe("_blank");
    expect(get.mock.calls.some((c) => c[0] === "/v1/companies/named-like")).toBe(false);
  });

  it("asks a linked company without a LinkedIn page for it first, and saves it", async () => {
    serve({ saved: { ...company, linkedinUrl: null } });
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(company));
    show({ companyId: "comp_1" });
    const form = await screen.findByRole("form", { name: "Its LinkedIn page" });
    expect(screen.queryByRole("region", { name: "Find HR" })).toBeNull();
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "https://www.linkedin.com/company/contoh-logistik/about/" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save page" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(patch.mock.calls[0]?.[0]).toBe("/v1/companies/{companyId}");
    expect(sentBody(patch)).toEqual({ version: 2, linkedinUrl: "https://www.linkedin.com/company/contoh-logistik/" });
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
  });

  it("says what is missing instead of offering what the user may not do", async () => {
    serve({ matches: [] });
    show({}, ["leads:read", "companies:read", "contacts:read"]);
    expect(await screen.findByText(/needs the permission to manage leads/)).toBeTruthy();
    expect(screen.queryByRole("form", { name: "New company" })).toBeNull();
    cleanup();
    serve();
    show({}, ["leads:read", "leads:manage", "companies:read", "contacts:read"]);
    fireEvent.click(await screen.findByRole("button", { name: /None of these/ }));
    expect(screen.getByText(/needs the permission to create companies/)).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /^This one/ })).toHaveLength(2);
    cleanup();
    serve({ saved: { ...company, linkedinUrl: null } });
    show({ companyId: "comp_1" }, ["leads:read", "companies:read", "contacts:read"]);
    expect(await screen.findByText(/adding it needs the permission to edit companies/)).toBeTruthy();
  });

  it("closes with Escape or its close button, but not on Escape in a dialog", async () => {
    serve();
    const onClose = vi.fn();
    show({}, ALL, onClose);
    await screen.findByText("Contoh Logistics Berhad");
    const dialog = document.createElement("dialog");
    document.body.append(dialog);
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    dialog.remove();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
