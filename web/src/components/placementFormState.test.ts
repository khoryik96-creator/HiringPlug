import { describe, expect, it } from "vitest";

import { createBody, feePreview, formFromPlacement, moneyBody, updateBody } from "./placementFormState";

const blank = formFromPlacement(undefined, "");

describe("placement form", () => {
  it("sends a percentage fee with the salary, thousands separators removed", () => {
    const f = { ...blank, startDate: "2026-12-01", salaryAmount: "8,500", salaryCurrency: "myr", feePercent: "20" };
    expect(createBody(f, true)).toEqual({
      startDate: "2026-12-01", guaranteeDays: 90,
      money: { salary: { amount: "8500", currency: "MYR", period: "month" }, feePercent: "20", feeAmount: null, feeCurrency: null },
    });
    expect(feePreview(f)).toBe("20400.00");
    expect(createBody(f, false)).toEqual({ startDate: "2026-12-01", guaranteeDays: 90 }); // no money without permission
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
});
