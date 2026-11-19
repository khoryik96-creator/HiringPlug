import { expect, it } from "vitest";

import { breakdown, cents, claimsCsv, currenciesOf, type Fact, formatTotals, fromCents, periodDates, statusTotal, totals } from "./claims";

it("adds up money in whole cents, per currency", () => {
  expect(cents("23.5")).toBe(2350);
  expect(fromCents(2350)).toBe("23.50");
  expect(totals([{ amount: "0.10", currency: "MYR" }, { amount: "0.20", currency: "MYR" }, { amount: "5.00", currency: "SGD" }, { amount: null, currency: "MYR" }]))
    .toEqual([{ currency: "SGD", amount: "5.00", count: 1 }, { currency: "MYR", amount: "0.30", count: 3 }]);
  expect(formatTotals([])).toBe("—");
});

it("knows the periods' days", () => {
  const today = new Date(2026, 10, 19);
  expect(periodDates("this_month", today)).toEqual({ from: "2026-11-01", to: "2026-11-30" });
  expect(periodDates("last_month", today)).toEqual({ from: "2026-10-01", to: "2026-10-31" });
  expect(periodDates("last_3_months", today)).toEqual({ from: "2026-09-01", to: "2026-11-30" });
  expect(periodDates("this_year", new Date(2026, 0, 3))).toEqual({ from: "2026-01-01", to: "2026-12-31" });
});

const fact = (over: Partial<Fact>): Fact => ({
  status: "submitted", receiptType: "food", claimType: "travel", linkKind: null, linkId: null, userId: "usr_a", month: "2026-11",
  currency: "MYR", count: 1, amount: "10.00", ...over,
});

it("breaks claims down by what counts as claimed, largest first", () => {
  const facts = [
    fact({ receiptType: "food", amount: "10.00" }), fact({ receiptType: "grab_fare", amount: "30.00", status: "paid" }),
    fact({ receiptType: "food", amount: "5.00", status: "approved", count: 2 }), fact({ receiptType: "drinks", amount: "99.00", status: "draft" }),
    fact({ receiptType: "food", amount: "1.00", status: "rejected" }), fact({ receiptType: "food", amount: "70.00", currency: "SGD" }),
  ];
  expect(breakdown(facts, "MYR", (f) => f.receiptType)).toEqual([
    { key: "grab_fare", cents: 3000, count: 1 }, { key: "food", cents: 1500, count: 3 },
  ]);
  expect(statusTotal(facts, "MYR", "draft")).toEqual({ cents: 9900, count: 1 });
  expect(currenciesOf(facts)).toEqual(["MYR", "SGD"]);
});

it("writes a CSV a spreadsheet cannot run", () => {
  const csv = claimsCsv([{
    id: "clm_1", userId: "usr_1", userName: "Aisyah", status: "approved", ocrStatus: "done", ocrError: null, fileName: "r.png",
    contentType: "image/png", sizeBytes: 1, receiptDate: "2026-11-03", amount: "23.00", currency: "MYR", merchant: "=HYPERLINK(\"x\")",
    receiptType: "grab_fare", claimType: "travel", purpose: "Meeting, \"Acme\"", link: { kind: "company", id: "comp_1", name: "Acme", detail: null },
    fromReceipt: [], missing: [], claimedAt: "2026-11-04T01:00:00Z", reviewedAt: null, reviewedByName: null, reviewNote: null, paidAt: null,
    version: 2, createdAt: "", updatedAt: "",
  }], { receiptType: () => "Grab / e-hailing fare", claimType: () => "Travel" });
  const [head, row] = csv.trim().split("\r\n");
  expect(head).toContain("Receipt date,Merchant");
  expect(row).toBe(`Aisyah,2026-11-03,"'=HYPERLINK(""x"")",Grab / e-hailing fare,Travel,Acme,"Meeting, ""Acme""",MYR,23.00,"Approved, to pay",2026-11-04,`);
});
