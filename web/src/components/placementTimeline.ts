// A placement's billing timeline: what happened and what is coming, by day
// (pure, unit tested).
import type { Schemas } from "../api/client";
import { formatMoney } from "../labels";

type Placement = Schemas["Placement"];

export type TimelineEntry = { day: string; label: string; state?: "paid" | "due" | "overdue" | "done" };

/** The placement's days in order: start, invoice, payment, guarantee and end. */
export function billingTimeline(p: Placement, today: string): TimelineEntry[] {
  const out: TimelineEntry[] = [{ day: p.startDate, label: "Starts" }];
  const inv = p.invoicing;
  const fee = p.money?.feeAmount && p.money.feeCurrency ? ` · ${formatMoney(p.money.feeAmount, p.money.feeCurrency)}` : "";
  if (p.invoicedOn) out.push({ day: p.invoicedOn, label: p.invoiceNumber ? `Invoiced · ${p.invoiceNumber}` : "Invoiced", state: "done" });
  else if (inv.sendsAt) out.push({ day: inv.sendsAt.slice(0, 10), label: "Invoice goes out" });
  if (p.paidOn) out.push({ day: p.paidOn, label: `Paid${fee}`, state: "paid" });
  else if (p.status !== "cancelled") out.push({ day: inv.dueOn, label: `Payment due${fee}`, state: inv.dueOn < today ? "overdue" : "due" });
  if (p.leftOn) out.push({ day: p.leftOn, label: "Left" });
  out.push({ day: p.guaranteeEndsOn, label: "Guarantee ends" });
  if (p.endDate) out.push({ day: p.endDate, label: p.jobType === "contract" ? "Contract ends" : "Ends" });
  return out.sort((a, b) => a.day.localeCompare(b.day));
}

/** "20.00" -> "20", "17.50" -> "17.5". */
export const trimZeros = (n: string) => (n.includes(".") ? n.replace(/0+$/, "").replace(/\.$/, "") : n);
