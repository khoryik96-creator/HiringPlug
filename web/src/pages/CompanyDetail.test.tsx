import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company } from "../api/client";
import { ok, RECRUITER, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CompanyDetail } from "./CompanyDetail";
import { CompanyList } from "./CompanyList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const company: Company = {
  id: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, name: "Grab", type: "prospect", category: "vip_client", linkedinUrl: "https://www.linkedin.com/company/grab",
  website: "https://grab.com", industry: "Technology", size: "large", phone: null, address: "1 Fusionopolis\nSingapore",
  city: "Singapore", country: "SG", registrationNumber: "201221800W", about: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};
const answers: Record<string, unknown> = {
  "/v1/companies/{companyId}": company,
  "/v1/companies/{companyId}/autofill": { status: "idle", checkedAt: null, error: null, suggestions: [] },
  "/v1/companies/{companyId}/benefits": { benefits: [], preOffer: [], version: 0, updatedAt: null },
  "/v1/settings/fields": testFieldSettings(),
  "/v1/companies/{companyId}/terms": {
    tiers: [
      { salaryFrom: null, salaryBelow: "120000", feePercent: "18", guaranteeDays: 60, paymentTermsDays: 14, remarks: null },
      { salaryFrom: "120000", salaryBelow: null, feePercent: "25", guaranteeDays: 90, paymentTermsDays: 30, remarks: null },
    ],
    salaryCurrency: "MYR", signedOn: null, notes: null, version: 1, updatedAt: null,
  },
};

describe("company pages", () => {
  it("shows a company in words, with an edit button for those who may edit", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(answers[path] ?? { items: [], nextCursor: null }))) as never);
    renderWithClient(<CompanyDetail id={company.id} permissions={RECRUITER.concat("companies:update")} />);
    await screen.findByRole("heading", { name: "Grab" });
    const card = screen.getByRole("region", { name: "Company" });
    expect(within(card).getByText("Prospect · Technology")).toBeTruthy();
    expect(within(card).getByText("Singapore, Singapore (SG)")).toBeTruthy();
    expect(within(card).getAllByText("VIP client").length, "its category, on the card and down its edge").toBeGreaterThanOrEqual(2);
    expect(await within(card).findByText("18–25%")).toBeTruthy();
    expect(within(card).getByText("by salary band")).toBeTruthy();
    expect(within(card).getByRole("button", { name: "VIP client" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getAllByText("201–1,000 people").length).toBeGreaterThan(0);
    expect(within(card).getByRole("link", { name: "company/grab" }).getAttribute("href")).toBe("https://www.linkedin.com/company/grab");
    expect(screen.getByRole("button", { name: "Edit company" })).toBeTruthy();

    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...company, category: "signed_client", version: 2 }));
    fireEvent.click(within(card).getByRole("button", { name: "Signed client" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 1, category: "signed_client" });
    expect(await screen.findByRole("button", { name: "Fill in from website" })).toBeTruthy();
  });

  it("lays its sections out as tabs, like a candidate's, and shows its billing to those who see fees", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/companies/{companyId}/billing"
      ? { year: 2026, thisYear: [{ currency: "MYR", total: "20000.00", notInvoiced: "20000.00", invoiced: "0.00", paid: "0.00" }],
        allTime: [{ currency: "MYR", total: "30000.00", notInvoiced: "30000.00", invoiced: "0.00", paid: "0.00" }] }
      : answers[path] ?? { items: [], nextCursor: null }))) as never);
    renderWithClient(<CompanyDetail id={company.id} permissions={RECRUITER.concat("placements:read", "placements.fees:read")} />);
    await screen.findByRole("heading", { name: "Grab" });
    const card = screen.getByRole("region", { name: "Company" });
    expect(await within(card).findByText("RM 20,000")).toBeTruthy();
    expect(within(card).getByText("Billing 2026"), "the agency's year, from the server").toBeTruthy();
    expect(within(card).getByText("RM 30,000 all time")).toBeTruthy();

    expect(screen.getAllByRole("tab").map((t) => t.textContent)).toEqual(["Details", "Terms & benefits", "Jobs", "Contacts", "Activity"]);
    expect(screen.queryByRole("region", { name: "Contacts" }), "a tab loads when opened").toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: "Contacts" }));
    expect(await screen.findByRole("region", { name: "Contacts" })).toBeTruthy();
  });

  it("keeps the card read-only while the edit form is open", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(answers[path] ?? { items: [], nextCursor: null }))) as never);
    renderWithClient(<CompanyDetail id={company.id} permissions={RECRUITER.concat("companies:update")} />);
    await screen.findByRole("group", { name: "Category" });
    fireEvent.click(screen.getByRole("button", { name: "Edit company" }));
    expect(screen.queryByRole("group", { name: "Category" }), "a category click would make the form's save stale").toBeNull();
  });

  it("leaves the billing out for those who may not see fees", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(answers[path] ?? { items: [], nextCursor: null }))) as never);
    renderWithClient(<CompanyDetail id={company.id} permissions={RECRUITER} />);
    await screen.findByRole("heading", { name: "Grab" });
    expect(screen.queryByText(/^Billing/)).toBeNull();
    expect(get.mock.calls.some((call) => (call as unknown[])[0] === "/v1/companies/{companyId}/billing")).toBe(false);
  });

  it("gives viewers no edit or add buttons", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(answers[path] ?? { items: [], nextCursor: null }))) as never);
    renderWithClient(<CompanyDetail id={company.id} permissions={["companies:read"]} />);
    await screen.findByRole("heading", { name: "Grab" });
    expect(screen.queryByRole("button", { name: "Edit company" })).toBeNull();
    expect(screen.queryByRole("group", { name: "Category" }), "changing the category is an edit").toBeNull();
    expect(screen.queryByRole("region", { name: "From the website" })).toBeNull();
    cleanup();

    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/settings/fields" ? testFieldSettings() : { items: [company], nextCursor: null }))) as never);
    renderWithClient(<CompanyList canCreate={false} />);
    await screen.findByRole("link", { name: "Grab" });
    expect(screen.queryByRole("button", { name: "Add company" })).toBeNull();
  });
});
