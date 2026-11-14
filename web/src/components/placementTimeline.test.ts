import { describe, expect, it } from "vitest";

import type { Schemas } from "../api/client";
import { billingTimeline, feeBasis, feeNote, invoiceBadge, monthsLong, trimZeros } from "./placementTimeline";

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
  it("puts the invoice on the day it goes out where the user is", () => {
    const tz = process.env.TZ;
    process.env.TZ = "Asia/Kuala_Lumpur";
    try {
      const t = billingTimeline({ ...base, invoicing: { ...base.invoicing, sendsAt: "2026-11-30T16:30:00Z" } }, "2026-11-12");
      expect(t.find((e) => e.label === "Invoice goes out")?.day).toBe("2026-12-01"); // 00:30 on 1 Dec in Kuala Lumpur
    } finally {
      process.env.TZ = tz;
    }
  });
  it("shows a payment as overdue, then paid", () => {
    expect(billingTimeline(base, "2027-01-02").find((e) => e.label === "Payment due")?.state).toBe("overdue");
    const paid = billingTimeline({ ...base, invoicedOn: "2026-12-01", paidOn: "2026-12-20", endDate: null, jobType: "permanent" }, "2027-01-02");
    expect(paid.map((e) => e.label)).toEqual(["Starts", "Invoiced", "Paid", "Guarantee ends"]);
  });
});

describe("with a retainer", () => {
  it("shows the stages and the balance due", () => {
    const stage = (label: string, dueOn: string | null, invoiceStatus: string, paidOn: string | null = null) =>
      ({ label, dueOn, invoiceStatus, paidOn, amount: "15000.00", dueKind: "on_date", dueDays: null, invoiceNumber: null, invoicedOn: null });
    const money = { ...base.money, retainer: { currency: "MYR", total: "30000.00", balance: "3600.00",
      stages: [stage("Engagement", "2026-11-01", "paid", "2026-11-05"), stage("Shortlist", "2026-11-10", "invoiced"), stage("Later", null, "not_invoiced")] } };
    const t = billingTimeline({ ...base, money } as unknown as Schemas["Placement"], "2026-11-12");
    expect(t.map((e) => `${e.day} ${e.label} ${e.state}`)).toEqual([
      "2026-11-05 Retainer · Engagement done", "2026-11-10 Retainer · Shortlist overdue",
      "2026-12-01 Starts later", "2026-12-01 Invoice goes out later", "2026-12-31 Balance due due", "2027-03-01 Guarantee ends later", "2027-11-30 Contract ends later",
    ]);
    expect(feeNote(money as Schemas["PlacementMoney"])).toBe("Retainer MYR 30,000.00 · balance MYR 3,600.00");
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

  it("an invoice sent before they pulled out is to cancel, not overdue", () => {
    expect(invoiceBadge({ ...base, status: "backed_out", invoiceStatus: "invoiced", invoiceNumber: "INV-1" }, "2027-01-01"))
      .toEqual({ state: "none", label: "Invoice to cancel", note: "INV-1 · not owed: send a credit note" });
    expect(invoiceBadge({ ...base, status: "backed_out" }, "2027-01-01").label).toBe("No invoice");
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
