import { describe, expect, it } from "vitest";

import type { Schemas } from "../api/client";
import { billingTimeline, feeBasis, invoiceBadge, monthsLong, trimZeros } from "./placementTimeline";

const base = {
  startDate: "2026-12-01", guaranteeEndsOn: "2027-03-01", status: "active", jobType: "contract", endDate: "2027-11-30",
  invoiceStatus: "not_invoiced", invoicedOn: null, invoiceNumber: null, paidOn: null, leftOn: null,
  money: { feeAmount: "33600.00", feeCurrency: "MYR", feePercent: "20.00", salary: { amount: "14000.00", currency: "MYR", period: "month" } },
  invoicing: { dueOn: "2026-12-31", sendsAt: "2026-12-01T01:00:00Z" },
} as unknown as Schemas["Placement"];

describe("billingTimeline", () => {
  it("puts the placement's days in order, done up to today", () => {
    const t = billingTimeline(base, "2026-12-05");
    expect(t.map((e) => e.label)).toEqual(["Starts", "Invoice goes out", "Payment due", "Guarantee ends", "Contract ends"]);
    expect(t.map((e) => e.state)).toEqual(["done", "done", "due", "later", "later"]);
  });
  it("shows a payment as overdue, then paid", () => {
    expect(billingTimeline(base, "2027-01-02").find((e) => e.label === "Payment due")?.state).toBe("overdue");
    const paid = billingTimeline({ ...base, invoicedOn: "2026-12-01", paidOn: "2026-12-20", endDate: null, jobType: "permanent" }, "2027-01-02");
    expect(paid.map((e) => e.label)).toEqual(["Starts", "Invoiced", "Paid", "Guarantee ends"]);
  });
});

describe("invoiceBadge", () => {
  it("says where the invoice stands", () => {
    expect(invoiceBadge(base, "2026-12-05")).toEqual({ state: "none", label: "Not invoiced", note: "due 31 Dec 2026" });
    expect(invoiceBadge({ ...base, invoiceStatus: "invoiced", invoiceNumber: "INV-1" }, "2026-12-05"))
      .toEqual({ state: "due", label: "Invoiced", note: "INV-1 · due 31 Dec 2026" });
    expect(invoiceBadge({ ...base, invoiceStatus: "invoiced" }, "2027-01-01").label).toBe("Overdue");
    expect(invoiceBadge({ ...base, invoiceStatus: "paid", paidOn: "2026-12-20" }, "2027-01-01")).toEqual({ state: "paid", label: "Paid", note: "20 Dec 2026" });
  });
});

describe("figures", () => {
  it("says how the fee was worked out and how long the placement runs", () => {
    const money = base.money as Schemas["PlacementMoney"];
    expect(feeBasis(money)).toBe("20% of MYR 14,000.00 / month");
    expect(feeBasis({ ...money, feePercent: null, salary: null })).toBe("Fixed fee");
    expect(monthsLong("2026-12-01", "2027-11-30")).toBe("12 months");
    expect(monthsLong("2026-12-01", "2027-01-31")).toBe("2 months");
    expect(monthsLong("2026-12-15", "2027-01-10")).toBe("under a month");
  });
  it("drops trailing zeros after the point only", () => {
    expect(["20.00", "17.50", "20", "100", "12.125"].map(trimZeros)).toEqual(["20", "17.5", "20", "100", "12.125"]);
  });
});
