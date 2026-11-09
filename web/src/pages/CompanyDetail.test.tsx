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
  id: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, name: "Grab", type: "prospect", category: "vip_client",
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
    expect(screen.getByRole("button", { name: "Edit company" })).toBeTruthy();

    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...company, category: "signed_client", version: 2 }));
    fireEvent.click(within(card).getByRole("button", { name: "Signed client" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 1, category: "signed_client" });
    expect(await screen.findByRole("button", { name: "Fill in from website" })).toBeTruthy();
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
