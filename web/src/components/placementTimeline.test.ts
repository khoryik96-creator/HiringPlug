import { describe, expect, it } from "vitest";

import type { Schemas } from "../api/client";
import { billingTimeline, trimZeros } from "./placementTimeline";

const base = {
  startDate: "2026-12-01", guaranteeEndsOn: "2027-03-01", status: "active", jobType: "contract", endDate: "2027-11-30",
  invoicedOn: null, invoiceNumber: null, paidOn: null, leftOn: null,
  money: { feeAmount: "33600.00", feeCurrency: "MYR", feePercent: "20", salary: null },
  invoicing: { dueOn: "2026-12-31", sendsAt: "2026-12-01T01:00:00Z" },
} as unknown as Schemas["Placement"];

describe("billingTimeline", () => {
  it("puts the placement's days in order", () => {
    const t = billingTimeline(base, "2026-11-12");
    expect(t.map((e) => e.label)).toEqual(["Starts", "Invoice goes out", "Payment due · MYR 33,600.00", "Guarantee ends", "Contract ends"]);
    expect(t[2]?.state).toBe("due");
  });
  it("shows a payment as overdue, then paid", () => {
    expect(billingTimeline(base, "2027-01-02").find((e) => e.label.startsWith("Payment due"))?.state).toBe("overdue");
    const paid = billingTimeline({ ...base, invoicedOn: "2026-12-01", invoiceNumber: "INV-0042", paidOn: "2026-12-20", endDate: null, jobType: "permanent" }, "2027-01-02");
    expect(paid.map((e) => e.label)).toEqual(["Starts", "Invoiced · INV-0042", "Paid · MYR 33,600.00", "Guarantee ends"]);
  });
});

describe("trimZeros", () => {
  it("drops trailing zeros after the point only", () => {
    expect(["20.00", "17.50", "20", "100", "12.125"].map(trimZeros)).toEqual(["20", "17.5", "20", "100", "12.125"]);
  });
});
