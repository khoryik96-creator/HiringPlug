// Words and sums for expense claims: statuses, fields, money per currency,
// periods, the dashboard's breakdowns and the CSV an approver downloads.
import type { Schemas } from "../../api/client";
import { formatMoney } from "../../labels";

export type Claim = Schemas["Claim"];
export type ClaimStatus = Schemas["ClaimStatus"];
export type ClaimField = Schemas["ClaimField"];
export type Fact = Schemas["ClaimFact"];

export const CLAIM_STATUS: Record<ClaimStatus, string> = {
  draft: "Draft", submitted: "Waiting for approval", approved: "Approved, to pay", rejected: "Rejected", paid: "Paid",
};

export const FIELD_LABEL: Record<ClaimField, string> = {
  receiptDate: "receipt date", amount: "amount", currency: "currency", merchant: "merchant", receiptType: "receipt type",
  claimType: "claim type", purpose: "purpose", link: "client or candidate",
};

/** Currencies offered (a receipt read in another keeps its own). */
export const CURRENCIES = ["MYR", "SGD", "USD", "IDR", "THB", "PHP", "VND", "EUR", "GBP", "AUD", "HKD", "CNY", "JPY", "INR"];

/** "23.50" as whole cents (no floating point). */
export function cents(amount: string): number {
  const [whole = "0", frac = ""] = amount.split(".");
  return Number(whole) * 100 + Number((frac + "00").slice(0, 2));
}

export function fromCents(n: number): string {
  return `${String(Math.floor(n / 100))}.${String(n % 100).padStart(2, "0")}`;
}

export type Total = { currency: string; amount: string; count: number };

/** Claims added up per currency, largest first. */
export function totals(claims: Pick<Claim, "amount" | "currency">[]): Total[] {
  const by = new Map<string, { cents: number; count: number }>();
  for (const c of claims) {
    const t = by.get(c.currency) ?? { cents: 0, count: 0 };
    t.cents += c.amount ? cents(c.amount) : 0;
    t.count += 1;
    by.set(c.currency, t);
  }
  return [...by.entries()].sort((a, b) => b[1].cents - a[1].cents)
    .map(([currency, t]) => ({ currency, amount: fromCents(t.cents), count: t.count }));
}

/** "MYR 1,234.50 + SGD 20.00" (nothing: "—"). */
export function formatTotals(list: { currency: string; amount: string }[]): string {
  return list.length === 0 ? "—" : list.map((t) => formatMoney(t.amount, t.currency)).join(" + ");
}

export type Period = "this_month" | "last_month" | "last_3_months" | "this_year";
export const PERIODS: [Period, string][] = [
  ["this_month", "This month"], ["last_month", "Last month"], ["last_3_months", "Last 3 months"], ["this_year", "This year"],
];

const day = (d: Date) => d.toISOString().slice(0, 10);

/** The first and last day (YYYY-MM-DD) of a period, for the day `today`. */
export function periodDates(period: Period, today: Date = new Date()): { from: string; to: string } {
  const y = today.getFullYear();
  const m = today.getMonth();
  const utc = (yy: number, mm: number, dd: number) => new Date(Date.UTC(yy, mm, dd));
  switch (period) {
    case "this_month": return { from: day(utc(y, m, 1)), to: day(utc(y, m + 1, 0)) };
    case "last_month": return { from: day(utc(y, m - 1, 1)), to: day(utc(y, m, 0)) };
    case "last_3_months": return { from: day(utc(y, m - 2, 1)), to: day(utc(y, m + 1, 0)) };
    case "this_year": return { from: day(utc(y, 0, 1)), to: day(utc(y, 11, 31)) };
  }
}

/** Statuses that count as claimed: sent for approval, approved or paid. */
export const CLAIMED: ClaimStatus[] = ["submitted", "approved", "paid"];

export type Row = { key: string; cents: number; count: number };

/** Facts in one currency added up by a dimension (only claimed ones unless `statuses` say), largest first. */
export function breakdown(facts: Fact[], currency: string, by: (f: Fact) => string | null | undefined, statuses: ClaimStatus[] = CLAIMED): Row[] {
  const rows = new Map<string, Row>();
  for (const f of facts) {
    if (f.currency !== currency || !statuses.includes(f.status)) continue;
    const key = by(f) ?? "";
    const row = rows.get(key) ?? { key, cents: 0, count: 0 };
    row.cents += cents(f.amount);
    row.count += f.count;
    rows.set(key, row);
  }
  return [...rows.values()].sort((a, b) => b.cents - a.cents || a.key.localeCompare(b.key));
}

/** The currencies in the facts, the one with the most claimed first. */
export function currenciesOf(facts: Fact[]): string[] {
  const sum = new Map<string, number>();
  for (const f of facts) sum.set(f.currency, (sum.get(f.currency) ?? 0) + cents(f.amount));
  return [...sum.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c);
}

/** One status's claims in a currency: how many and how much. */
export function statusTotal(facts: Fact[], currency: string, status: ClaimStatus): { cents: number; count: number } {
  let total = 0;
  let count = 0;
  for (const f of facts) {
    if (f.currency === currency && f.status === status) {
      total += cents(f.amount);
      count += f.count;
    }
  }
  return { cents: total, count };
}

/** Claims as CSV (Excel opens it), one per row. */
export function claimsCsv(claims: Claim[], word: { receiptType: (k: string) => string; claimType: (k: string) => string }): string {
  const cell = (v: string | number | null | undefined) => {
    const s = v === null || v === undefined ? "" : String(v);
    const safe = /^[=+\-@]/.test(s) ? `'${s}` : s; // never a formula in a spreadsheet
    return /[",\n]/.test(safe) ? `"${safe.replaceAll('"', '""')}"` : safe;
  };
  const head = ["Person", "Receipt date", "Merchant", "Receipt type", "Claim type", "Client or candidate", "Purpose", "Currency", "Amount", "Status", "Claimed on", "Paid on"];
  const rows = claims.map((c) => [
    c.userName, c.receiptDate, c.merchant, c.receiptType ? word.receiptType(c.receiptType) : "", c.claimType ? word.claimType(c.claimType) : "",
    c.link?.name ?? "", c.purpose, c.currency, c.amount, CLAIM_STATUS[c.status], c.claimedAt?.slice(0, 10), c.paidAt?.slice(0, 10),
  ].map(cell).join(","));
  return [head.join(","), ...rows].join("\r\n") + "\r\n";
}
