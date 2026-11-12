import { describe, expect, it } from "vitest";

import type { Schemas } from "../api/client";
import { createBody, feePreview, formFromPlacement, moneyBody, updateBody, withDefaults } from "./placementFormState";

const blank = formFromPlacement(undefined, "");

describe("placement form", () => {
  it("sends a percentage fee with the salary, thousands separators removed", () => {
    const f = { ...blank, startDate: "2026-12-01", salaryAmount: "8,500", salaryCurrency: "myr", feePercent: "20" };
    expect(createBody(f, true)).toEqual({
      startDate: "2026-12-01", guaranteeDays: 90, jobType: "permanent",
      money: { salary: { amount: "8500", currency: "MYR", period: "month" }, feePercent: "20", feeAmount: null, feeCurrency: null },
    });
    expect(feePreview(f)).toBe("20400.00");
    expect(createBody(f, false)).toEqual({ startDate: "2026-12-01", guaranteeDays: 90, jobType: "permanent" }); // no money without permission
  });

  it("sends a fixed fee, or nothing when no money is entered", () => {
    expect(moneyBody({ ...blank, feeKind: "fixed", feeAmount: "15000", feeCurrency: "sgd" }))
      .toEqual({ salary: null, feePercent: null, feeAmount: "15000", feeCurrency: "SGD" });
    expect(moneyBody(blank)).toBeNull();
    expect(feePreview({ ...blank, salaryAmount: "50", salaryPeriod: "hour", feePercent: "20" })).toBeNull();
  });

  it("only sends what the status and invoice state use", () => {
    const f = { ...blank, startDate: "2026-12-01", leftOn: "2027-01-02", invoiceStatus: "invoiced" as const, invoiceNumber: " INV-1 ", paidOn: "2027-01-05" };
    const body = updateBody(f, 3, false);
    expect(body).toMatchObject({ version: 3, status: "active", leftOn: null, invoiceNumber: "INV-1", paidOn: null, recruiterId: null });
    expect(body).not.toHaveProperty("money");
  });

  it("starts the sheet from the job, and sends what was changed", () => {
    const d = { jobTitle: "Finance Manager", companyName: "Maju Foods", jobType: "contract", guaranteeDays: 90, paymentTermsDays: 30, contactId: null, feePercent: null } as unknown as Schemas["PlacementDefaults"];
    const f = withDefaults(blank, d, false);
    expect(f).toMatchObject({ jobTitle: "Finance Manager", companyName: "Maju Foods", jobType: "contract" });
    expect(createBody({ ...f, startDate: "2026-12-01", poNumber: " PO-1 ", endDate: "2027-11-30" }, false)).toMatchObject({
      jobTitle: "Finance Manager", companyName: "Maju Foods", jobType: "contract", poNumber: "PO-1", endDate: "2027-11-30",
    });
    expect(updateBody({ ...f, jobTitle: " ", poNumber: "" }, 2, false)).toMatchObject({ jobTitle: "", poNumber: null, endDate: null });
  });
});
