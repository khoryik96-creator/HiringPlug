import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { CompanyTerms } from "./CompanyTerms";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const none = { tiers: [], salaryCurrency: "MYR", signedOn: null, notes: null, version: 0, updatedAt: null };
const rate = (salaryFrom: string | null, salaryBelow: string | null, feePercent: string, guaranteeDays: number, paymentTermsDays: number, remarks: string | null = null) =>
  ({ salaryFrom, salaryBelow, feePercent, guaranteeDays, paymentTermsDays, remarks });
const tiered = {
  tiers: [rate("120000", null, "25", 90, 30, "50% on signing, 50% on start"), rate(null, "120000", "18", 60, 14)],
  salaryCurrency: "MYR", signedOn: "2026-01-15", notes: "One free replacement.", version: 1, updatedAt: "2026-11-09T03:00:00Z",
};
const reads = (terms: object) => vi.spyOn(api, "GET").mockImplementation(((path: string) =>
  Promise.resolve(ok(path === "/v1/companies/{companyId}/terms" ? terms : { items: [] }))) as never);

describe("a client's terms of business", () => {
  it("are tiers by annual salary, each with a fee, guarantee, payment terms and payment remarks", async () => {
    reads(none);
    const put = vi.spyOn(api, "PUT").mockResolvedValueOnce(invalidField("tiers[1].salaryFrom", "overlaps rate 1")).mockResolvedValue(ok(tiered));
    renderWithClient(<CompanyTerms companyId="comp_1" permissions={["companies:read", "companies:update"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Add terms" }));
    fireEvent.change(screen.getByLabelText("Rate 1: annual salary from"), { target: { value: "120,000" } });
    fireEvent.change(screen.getByLabelText("Rate 1: fee %"), { target: { value: "25%" } });
    fireEvent.change(screen.getByLabelText("Rate 1: payment method / remarks"), { target: { value: "50% on signing, 50% on start" } });
    fireEvent.click(screen.getByRole("button", { name: "+ Add a salary band" }));
    fireEvent.change(screen.getByLabelText("Rate 2: below"), { target: { value: "RM120000" } });
    fireEvent.change(screen.getByLabelText("Rate 2: fee %"), { target: { value: "18" } });
    fireEvent.change(screen.getByLabelText("Rate 2: guarantee"), { target: { value: "60" } });
    fireEvent.change(screen.getByLabelText("Rate 2: guarantee in"), { target: { value: "days" } });
    fireEvent.change(screen.getByLabelText("Rate 2: payment terms (days)"), { target: { value: "14" } });
    fireEvent.click(screen.getByRole("button", { name: "+ Add a salary band" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove rate 3" }));
    fireEvent.click(screen.getByRole("button", { name: "Save terms" }));
    expect(await screen.findByText("Rate 2: overlaps rate 1")).toBeTruthy();
    expect(sentBody(put)).toEqual({
      version: 0, salaryCurrency: "MYR", signedOn: null, notes: null,
      tiers: [rate("120000", null, "25", 90, 30, "50% on signing, 50% on start"), rate(null, "120000", "18", 60, 14)],
    });
    fireEvent.click(screen.getByRole("button", { name: "Save terms" }));
    await waitFor(() => { expect(screen.getByText("RM 120,000 and above")).toBeTruthy(); });
    expect(screen.getByText("Below RM 120,000")).toBeTruthy();
    expect(screen.getByText("3 months")).toBeTruthy();
    expect(screen.getByText("14 days")).toBeTruthy();
    expect(screen.getByText("50% on signing, 50% on start")).toBeTruthy();
  });

  it("are read only without companies:update", async () => {
    reads(tiered);
    renderWithClient(<CompanyTerms companyId="comp_1" permissions={["companies:read"]} />);
    expect(await screen.findByText("2 months")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });
});
