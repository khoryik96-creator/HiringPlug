// A placement at a glance: its billing timeline, where the invoice stands and
// how the fee was worked out (pure, unit tested).
import type { Schemas } from "../api/client";
import { PAY_PERIOD, formatDay, formatMoney } from "../labels";

type Placement = Schemas["Placement"];

export type TimelineEntry = { day: string; label: string; state: "done" | "due" | "overdue" | "later" };

/** The placement's days in order: start, invoice, payment, guarantee and end. */
export function billingTimeline(p: Placement, today: string): TimelineEntry[] {
  const at = (day: string, label: string): TimelineEntry => ({ day, label, state: day <= today ? "done" : "later" });
  const inv = p.invoicing;
  const out: TimelineEntry[] = [at(p.startDate, "Starts")];
  if (p.invoicedOn) out.push({ day: p.invoicedOn, label: "Invoiced", state: "done" });
  else if (inv.sendsAt) out.push(at(localDay(inv.sendsAt), "Invoice goes out"));
  if (p.paidOn) out.push({ day: p.paidOn, label: "Paid", state: "done" });
  else if (p.status !== "cancelled") out.push({ day: inv.dueOn, label: "Payment due", state: inv.dueOn < today ? "overdue" : "due" });
  if (p.leftOn) out.push(at(p.leftOn, "Left"));
  out.push(at(p.guaranteeEndsOn, "Guarantee ends"));
  if (p.endDate) out.push(at(p.endDate, p.jobType === "contract" ? "Contract ends" : "Ends"));
  return out.sort((a, b) => a.day.localeCompare(b.day));
}

export type InvoiceBadge = { state: "paid" | "due" | "overdue" | "none"; label: string; note: string };

/** Where the invoice stands, as a pill and a line under it. */
export function invoiceBadge(p: Placement, today: string): InvoiceBadge {
  const due = `due ${formatDay(p.invoicing.dueOn)}`;
  if (p.status === "cancelled" && p.invoiceStatus === "not_invoiced") return { state: "none", label: "No invoice", note: "" };
  if (p.invoiceStatus === "paid") return { state: "paid", label: "Paid", note: p.paidOn ? formatDay(p.paidOn) : "" };
  if (p.invoiceStatus === "invoiced") {
    const late = p.invoicing.dueOn < today;
    return { state: late ? "overdue" : "due", label: late ? "Overdue" : "Invoiced", note: [p.invoiceNumber, due].filter(Boolean).join(" · ") };
  }
  return { state: "none", label: "Not invoiced", note: due };
}

/** "20% of MYR 14,000.00 / month", or "Fixed fee". */
export function feeBasis(m: Schemas["PlacementMoney"]): string {
  if (m.feePercent && m.salary) return `${trimZeros(m.feePercent)}% of ${formatMoney(m.salary.amount, m.salary.currency)} / ${PAY_PERIOD[m.salary.period].toLowerCase()}`;
  return m.feeAmount ? "Fixed fee" : "";
}

/** Whole months from one day to another ("12 months"), for a placement's length. */
export function monthsLong(start: string, end: string): string {
  const s = new Date(`${start}T00:00:00Z`);
  const e = new Date(`${end}T00:00:00Z`);
  e.setUTCDate(e.getUTCDate() + 1); // the end day counts
  const months = (e.getUTCFullYear() - s.getUTCFullYear()) * 12 + e.getUTCMonth() - s.getUTCMonth() - (e.getUTCDate() < s.getUTCDate() ? 1 : 0);
  if (months < 1) return "under a month";
  return months === 1 ? "1 month" : `${String(months)} months`;
}

/** "20.00" -> "20", "17.50" -> "17.5". */
export const trimZeros = (n: string) => (n.includes(".") ? n.replace(/0+$/, "").replace(/\.$/, "") : n);

/** The day (YYYY-MM-DD) of a moment, in the user's time zone: today by default. */
export function localDay(iso?: string): string {
  const d = iso ? new Date(iso) : new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${String(d.getFullYear())}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
export const todayISO = () => localDay();
