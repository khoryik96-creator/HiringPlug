// A client's rates by annual salary (pure, unit tested): how a rate reads,
// which rate a salary falls in, and the terms form's rows.
import type { Schemas } from "../api/client";

export type Tier = Schemas["CompanyTermsTier"];

const money = (currency: string, amount: string) =>
  `${currency === "MYR" ? "RM" : currency} ${Number(amount).toLocaleString("en-MY", { maximumFractionDigits: 2 })}`;

/** "RM 120,000 and above", "Below RM 120,000", "Any salary". */
export function rangeLabel(t: Pick<Tier, "salaryFrom" | "salaryBelow">, currency: string): string {
  if (t.salaryFrom && t.salaryBelow) return `${money(currency, t.salaryFrom)} to below ${money(currency, t.salaryBelow)}`;
  if (t.salaryFrom) return `${money(currency, t.salaryFrom)} and above`;
  if (t.salaryBelow) return `Below ${money(currency, t.salaryBelow)}`;
  return "Any salary";
}

/** "3 months" for 90 days, "45 days" otherwise. */
export function guaranteeLabel(days: number | null): string {
  if (days === null) return "—";
  if (days > 0 && days % 30 === 0) return days === 30 ? "1 month" : `${String(days / 30)} months`;
  return `${String(days)} days`;
}

/** The rates depend on the salary (not one rate for any salary). */
export const bySalary = (tiers: Tier[]) => tiers.some((t) => t.salaryFrom !== null || t.salaryBelow !== null);

/**
 * The rate a salary falls in: its yearly amount, in the rates' currency,
 * from salaryFrom up to (not including) salaryBelow. With no salary, or one in
 * another currency, the one rate for any salary, if that is all there is.
 */
export function pickTier(tiers: Tier[], currency: string, salary: { amount: string; currency: string; period: string } | null): number {
  const amount = salary ? Number(salary.amount.replace(/[,\s]/g, "")) : NaN;
  if (salary && salary.currency.trim().toUpperCase() === currency && amount > 0) {
    const yearly = salary.period === "month" ? amount * 12 : amount;
    return tiers.findIndex((t) =>
      (t.salaryFrom === null || yearly >= Number(t.salaryFrom)) && (t.salaryBelow === null || yearly < Number(t.salaryBelow)));
  }
  return tiers.length === 1 && !bySalary(tiers) ? 0 : -1;
}

/** A rate as the terms form edits it; guarantee in days or months (30 days). */
export type TierRow = { from: string; below: string; fee: string; guarantee: string; unit: "days" | "months"; payment: string; remarks: string };

export const emptyRow = (): TierRow => ({ from: "", below: "", fee: "", guarantee: "3", unit: "months", payment: "30", remarks: "" });

export function rowFromTier(t: Tier): TierRow {
  const g = t.guaranteeDays;
  const months = g !== null && g > 0 && g % 30 === 0;
  return {
    from: t.salaryFrom ?? "", below: t.salaryBelow ?? "", fee: t.feePercent ?? "",
    guarantee: g === null ? "" : String(months ? g / 30 : g), unit: months || g === null ? "months" : "days",
    payment: t.paymentTermsDays?.toString() ?? "", remarks: t.remarks ?? "",
  };
}

const amount = (s: string) => s.replace(/[,\s]/g, "").replace(/^RM/i, "") || null;
const whole = (s: string) => (s.trim() === "" ? null : Number(s));

export function tierFromRow(r: TierRow): Tier {
  const g = whole(r.guarantee);
  return {
    salaryFrom: amount(r.from), salaryBelow: amount(r.below), feePercent: r.fee.trim().replace(/%$/, "") || null,
    guaranteeDays: g === null ? null : r.unit === "months" ? g * 30 : g, paymentTermsDays: whole(r.payment), remarks: r.remarks.trim() || null,
  };
}
