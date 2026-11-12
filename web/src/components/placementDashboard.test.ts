import { expect, it } from "vitest";

import type { Schemas } from "../api/client";
import { addDays, DEFAULT_VIEW, filtered, guarantee, onCard, rangeDates } from "./placementDashboard";

const today = "2026-12-10";
const p = (over: Partial<Schemas["Placement"]> = {}): Schemas["Placement"] => ({
  id: "plc_1", version: 1, applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti", jobId: "jo_1", jobTitle: "Accountant",
  companyId: "comp_1", companyName: "Contoh", jobType: "permanent", poNumber: null, endDate: null, recruiterId: "usr_a", recruiterName: "Aina",
  invoicing: { contactId: null, contactName: null, contactEmail: null, paymentTermsDays: 30, dueOn: "2026-12-31", dueOnSet: false, autoSend: false, sendsAt: null, sentAt: null, sendError: null },
  startDate: "2026-10-01", guaranteeDays: 90, guaranteeEndsOn: "2026-12-30", status: "active", leftOn: null, statusNote: null, replacement: "none",
  invoiceStatus: "not_invoiced", invoiceNumber: null, invoicedOn: null, paidOn: null, money: null, restrictedFields: [],
  createdAt: "2026-10-01T00:00:00Z", updatedAt: "2026-10-01T00:00:00Z", ...over,
});

it("tells where a guarantee stands, at its edges", () => {
  expect(guarantee(p({ guaranteeEndsOn: addDays(today, 30) }), today)).toEqual({ state: "ending_soon", note: "30 days left" });
  expect(guarantee(p({ guaranteeEndsOn: addDays(today, 31) }), today).state).toBe("in_guarantee");
  expect(guarantee(p({ guaranteeEndsOn: today }), today)).toEqual({ state: "ending_soon", note: "ends today" });
  expect(guarantee(p({ guaranteeEndsOn: addDays(today, -1) }), today).state).toBe("ended");
  expect(guarantee(p({ startDate: addDays(today, 1) }), today)).toEqual({ state: "not_started", note: "starts tomorrow" });
  expect(guarantee(p({ startDate: today, guaranteeEndsOn: addDays(today, 90) }), today).state, "starting today: in guarantee").toBe("in_guarantee");
  expect(guarantee(p({ status: "fell_through", leftOn: "2026-11-02" }), today)).toEqual({ state: "fell_through", note: "left 2 Nov 2026" });
  expect(guarantee(p({ status: "cancelled" }), today).state).toBe("cancelled");
});

it("counts each card as its label says", () => {
  expect(onCard(p({ invoiceStatus: "invoiced", invoicing: { ...p().invoicing, dueOn: addDays(today, -1) } }), "overdue", today)).toBe(true);
  expect(onCard(p({ invoiceStatus: "invoiced", invoicing: { ...p().invoicing, dueOn: today } }), "overdue", today), "due today is not overdue").toBe(false);
  expect(onCard(p({ invoiceStatus: "paid", invoicing: { ...p().invoicing, dueOn: "2026-01-01" } }), "overdue", today)).toBe(false);
  expect(onCard(p({ status: "cancelled" }), "not_invoiced", today), "a no-start owes no invoice").toBe(false);
  expect(onCard(p(), "not_invoiced", today)).toBe(true);
  expect(onCard(p({ status: "fell_through", replacement: "owed" }), "replacement", today)).toBe(true);
  expect(onCard(p({ status: "fell_through" }), "in_guarantee", today)).toBe(false);
});

it("filters by the placement's own recruiter, searches and sorts", () => {
  const items = [
    p({ id: "a", candidateName: "Ali", recruiterId: "usr_a", money: { salary: null, feePercent: null, feeAmount: "9000.00", feeCurrency: "MYR", retainer: null } }),
    p({ id: "b", candidateName: "Bea", recruiterId: "usr_b", companyName: "Maju Logistics", money: { salary: null, feePercent: null, feeAmount: "15000.00", feeCurrency: "MYR", retainer: null } }),
    p({ id: "c", candidateName: "Chen", recruiterId: null, recruiterName: null }),
  ];
  expect(filtered(items, { ...DEFAULT_VIEW, recruiter: "usr_b" }, today).map((x) => x.id)).toEqual(["b"]);
  expect(filtered(items, { ...DEFAULT_VIEW, q: "maju" }, today).map((x) => x.id)).toEqual(["b"]);
  expect(filtered(items, { ...DEFAULT_VIEW, sort: "fee_desc" }, today).map((x) => x.id), "no fee last").toEqual(["b", "a", "c"]);
  expect(filtered(items, { ...DEFAULT_VIEW, sort: "candidate" }, today).map((x) => x.id)).toEqual(["a", "b", "c"]);
});

it("turns the date presets into start dates, inclusive", () => {
  expect(rangeDates(DEFAULT_VIEW, today)).toEqual({ from: "2026-01-01", to: "2026-12-31" });
  expect(rangeDates({ ...DEFAULT_VIEW, range: "last_year" }, today)).toEqual({ from: "2025-01-01", to: "2025-12-31" });
  expect(rangeDates({ ...DEFAULT_VIEW, range: "all" }, today)).toEqual({ from: "", to: "" });
  expect(rangeDates({ ...DEFAULT_VIEW, range: "custom", from: "2026-03-01", to: "2026-03-31" }, today)).toEqual({ from: "2026-03-01", to: "2026-03-31" });
});
