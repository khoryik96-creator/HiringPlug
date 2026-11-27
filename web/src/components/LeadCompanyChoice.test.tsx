import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company, type Schemas } from "../api/client";
import * as companion from "../companion";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { LeadCompanyChoice } from "./LeadCompanyChoice";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const lead: Schemas["JobLead"] = {
  id: "lead_1", version: 4, sourceId: null, sourceKind: null, companyName: "Maju Foods Berhad", companyId: null,
  title: "HR Executive", location: "Petaling Jaya, MY", url: null, postedAt: null,
  firstSeenAt: "2026-11-01T00:00:00Z", lastSeenAt: "2026-11-03T00:00:00Z", closedAt: null, status: "new", notes: null,
};
const ALL = ["leads:read", "leads:manage", "companies:read", "companies:create", "contacts:read"] as const;
const page = "https://www.linkedin.com/company/maju-foods/";

/** The companies API: named-like answers from `named()`, and `byPage` for a LinkedIn page lookup. */
function serve({ named = () => [] as Schemas["CompanyNamedLike"][], byPage = [] as { id: string; name: string }[] } = {}) {
  vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: false, version: null });
  return vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: Record<string, unknown> } }) => Promise.resolve(ok(
    path === "/v1/companies/named-like" ? { items: named() }
      : path === "/v1/companies" && init?.params?.query?.linkedinCompany ? { items: byPage } : { items: [] },
  ))) as never);
}

const show = (onLinked = vi.fn()) => {
  renderWithClient(<LeadCompanyChoice lead={lead} permissions={[...ALL]} onLinked={onLinked} />);
  return onLinked;
};

describe("a lead's company, step 1", () => {
  it("links the company in HyPlug that has the LinkedIn page chosen, under another name, instead of adding a second", async () => {
    const get = serve({ byPage: [{ id: "comp_mfb", name: "MFB Group" }] });
    const post = vi.spyOn(api, "POST");
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, version: 5, companyId: "comp_mfb" }));
    const onLinked = show();
    const form = await screen.findByRole("form", { name: "New company" });
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "linkedin.com/company/maju-foods" } });
    fireEvent.click(within(form).getByRole("button", { name: "Add and link" }));
    expect(await within(form).findByText("MFB Group in HyPlug has this LinkedIn page.")).toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/companies", { params: { query: { linkedinCompany: "maju-foods", limit: 1 } } });
    expect(post).not.toHaveBeenCalled();
    expect(patch).not.toHaveBeenCalled();
    expect(within(form).getByRole<HTMLButtonElement>("button", { name: "Add and link" }).disabled).toBe(true);
    fireEvent.click(within(form).getByRole("button", { name: "Link the lead to MFB Group" }));
    await waitFor(() => { expect(onLinked).toHaveBeenCalledWith("comp_mfb"); });
    expect(sentBody(patch)).toEqual({ version: 4, companyId: "comp_mfb" });
    expect(post).not.toHaveBeenCalled();
    // Another page: adding it is offered again.
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "linkedin.com/company/maju-foods-berhad" } });
    expect(within(form).queryByText(/has this LinkedIn page/)).toBeNull();
  });

  it("keeps the company added, its name and page fixed, and why the link failed, then only links it", async () => {
    let added = false;
    const created = { id: "comp_new", version: 1, name: "Maju Food", linkedinUrl: page } as Company;
    // Once added, the company comes back from named-like with the same name.
    serve({ named: () => (added ? [{ id: "comp_new", name: "Maju Food", city: null, website: null, linkedinUrl: page, industry: null, sameName: true }] : []) });
    const post = vi.spyOn(api, "POST").mockImplementation((() => { added = true; return Promise.resolve(ok(created)); }) as never);
    const patch = vi.spyOn(api, "PATCH")
      .mockResolvedValueOnce(failure("CONFLICT_STALE_VERSION", "This lead was changed by someone else. Reload it and try again.", undefined, 409))
      .mockResolvedValueOnce(ok({ ...lead, version: 6, companyId: "comp_new" }));
    const onLinked = show();
    const form = await screen.findByRole("form", { name: "New company" });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Maju Food" } });
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: page } });
    fireEvent.click(within(form).getByRole("button", { name: "Add and link" }));
    expect(await within(form).findByText(/changed by someone else/)).toBeTruthy();
    // The matches read again (the new company, same name): the form and its error stay.
    await screen.findByRole("button", { name: "This one: Maju Food" });
    expect(screen.getByRole("form", { name: "New company" })).toBe(form);
    expect(within(form).getByText(/changed by someone else/)).toBeTruthy();
    expect(within(form).getByRole("heading", { name: "Maju Food is added to HyPlug" })).toBeTruthy();
    expect(within(form).getByLabelText<HTMLInputElement>("Name").disabled).toBe(true);
    expect(within(form).getByLabelText<HTMLInputElement>("LinkedIn page").disabled).toBe(true);
    fireEvent.click(within(form).getByRole("button", { name: "Link to Maju Food" }));
    await waitFor(() => { expect(onLinked).toHaveBeenCalledWith("comp_new"); });
    expect(post).toHaveBeenCalledTimes(1);
    expect(sentBody(patch, 1)).toEqual({ version: 4, companyId: "comp_new" });
  });

  it("offers to try the lookup again, and adding the company, when finding the companies with the name fails", async () => {
    let fails = true;
    vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: false, version: null });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/companies/named-like" && fails
      ? failure("INTERNAL", "Something went wrong on our side.", undefined, 500)
      : ok({ items: path === "/v1/companies/named-like" ? [{ id: "comp_1", name: "Maju Foods Berhad", city: null, website: null, linkedinUrl: null, industry: null, sameName: true }] : [] }))) as never);
    show();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Something went wrong on our side.");
    expect(screen.getByRole("form", { name: "New company" })).toBeTruthy();
    fails = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("button", { name: "This one: Maju Foods Berhad" })).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows why a link failed beside the button used, even while the lookup's own error shows", async () => {
    serve({ named: () => [{ id: "comp_1", name: "Maju Foods Berhad", city: null, website: null, linkedinUrl: null, industry: null, sameName: true }] });
    vi.spyOn(api, "PATCH").mockResolvedValue(failure("CONFLICT_STALE_VERSION", "This lead was changed by someone else. Reload it and try again.", undefined, 409));
    show();
    fireEvent.click(await screen.findByRole("button", { name: "This one: Maju Foods Berhad" }));
    const row = screen.getByRole("button", { name: "This one: Maju Foods Berhad" }).closest("li") as HTMLElement;
    expect(await within(row).findByText(/changed by someone else/)).toBeTruthy();
    cleanup();
    // The lookup failed, and the LinkedIn page's owner is linked from the form.
    vi.restoreAllMocks();
    vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: false, version: null });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/companies/named-like"
      ? failure("INTERNAL", "Something went wrong on our side.", undefined, 500)
      : ok({ items: [{ id: "comp_mfb", name: "MFB Group" }] }))) as never);
    vi.spyOn(api, "PATCH").mockResolvedValue(failure("CONFLICT_STALE_VERSION", "This lead was changed by someone else. Reload it and try again.", undefined, 409));
    show();
    const form = await screen.findByRole("form", { name: "New company" });
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: page } });
    fireEvent.click(within(form).getByRole("button", { name: "Add and link" }));
    fireEvent.click(await within(form).findByRole("button", { name: "Link the lead to MFB Group" }));
    const owner = within(form).getByText("MFB Group in HyPlug has this LinkedIn page.").closest("div") as HTMLElement;
    expect(await within(owner).findByText(/changed by someone else/)).toBeTruthy();
    expect(screen.getByText("Something went wrong on our side.")).toBeTruthy();
  });
});

