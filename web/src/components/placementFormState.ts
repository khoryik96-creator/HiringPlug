// The placement form's state and the request bodies built from it (pure, unit tested).
import type { Schemas } from "../api/client";

type Placement = Schemas["Placement"];

export type PlacementFormState = {
  startDate: string;
  guaranteeDays: string;
  recruiterId: string;
  status: Schemas["PlacementStatus"];
  leftOn: string;
  statusNote: string;
  replacement: Schemas["ReplacementStatus"];
  invoiceStatus: Schemas["InvoiceStatus"];
  invoiceNumber: string;
  invoicedOn: string;
  paidOn: string;
  salaryAmount: string;
  salaryCurrency: string;
  salaryPeriod: Schemas["PayPeriod"];
  feeKind: "percent" | "fixed";
  feePercent: string;
  feeAmount: string;
  feeCurrency: string;
};

export function formFromPlacement(p: Placement | undefined, recruiterId: string): PlacementFormState {
  const money = p?.money ?? null;
  return {
    startDate: p?.startDate ?? "",
    guaranteeDays: String(p?.guaranteeDays ?? 90),
    recruiterId: p ? (p.recruiterId ?? "") : recruiterId,
    status: p?.status ?? "active",
    leftOn: p?.leftOn ?? "",
    statusNote: p?.statusNote ?? "",
    replacement: p?.replacement ?? "none",
    invoiceStatus: p?.invoiceStatus ?? "not_invoiced",
    invoiceNumber: p?.invoiceNumber ?? "",
    invoicedOn: p?.invoicedOn ?? "",
    paidOn: p?.paidOn ?? "",
    salaryAmount: money?.salary?.amount ?? "",
    salaryCurrency: money?.salary?.currency ?? "MYR",
    salaryPeriod: money?.salary?.period ?? "month",
    feeKind: money && money.feePercent === null && money.feeAmount !== null ? "fixed" : "percent",
    feePercent: money?.feePercent ?? "",
    feeAmount: money?.feeAmount ?? "",
    feeCurrency: money?.feeCurrency ?? "MYR",
  };
}

/** "8,500" -> "8500": people type thousands separators. */
const amount = (s: string) => s.replace(/[,\s]/g, "");
const orNull = (s: string) => (s.trim() === "" ? null : s.trim());

/** The salary and fee, or null when none is entered. */
export function moneyBody(f: PlacementFormState): Schemas["PlacementMoneyInput"] | null {
  const salary = amount(f.salaryAmount)
    ? { amount: amount(f.salaryAmount), currency: f.salaryCurrency.trim().toUpperCase(), period: f.salaryPeriod }
    : null;
  const percent = f.feeKind === "percent" ? orNull(amount(f.feePercent)) : null;
  const fixed = f.feeKind === "fixed" ? orNull(amount(f.feeAmount)) : null;
  if (!salary && !percent && !fixed) return null;
  return {
    salary,
    feePercent: percent,
    feeAmount: fixed,
    feeCurrency: fixed ? f.feeCurrency.trim().toUpperCase() : null,
  };
}

/** What a percentage fee comes to, for a preview (the server works out the real figure). */
export function feePreview(f: PlacementFormState): string | null {
  const salary = Number(amount(f.salaryAmount));
  const pct = Number(amount(f.feePercent));
  if (f.feeKind !== "percent" || !salary || !pct || (f.salaryPeriod !== "month" && f.salaryPeriod !== "year")) return null;
  return ((salary * (f.salaryPeriod === "month" ? 12 : 1) * pct) / 100).toFixed(2);
}

export function createBody(f: PlacementFormState, withMoney: boolean): Schemas["PlacementInput"] {
  const money = withMoney ? moneyBody(f) : null;
  return {
    startDate: f.startDate,
    guaranteeDays: Number(f.guaranteeDays),
    ...(f.recruiterId ? { recruiterId: f.recruiterId } : {}),
    ...(money ? { money } : {}),
  };
}

export function updateBody(f: PlacementFormState, version: number, withMoney: boolean): Schemas["PlacementUpdate"] {
  return {
    version,
    startDate: f.startDate,
    guaranteeDays: Number(f.guaranteeDays),
    recruiterId: orNull(f.recruiterId),
    status: f.status,
    leftOn: f.status === "fell_through" ? orNull(f.leftOn) : null,
    statusNote: orNull(f.statusNote),
    ...(f.status === "fell_through" && f.replacement !== "none" ? { replacement: f.replacement } : {}),
    invoiceStatus: f.invoiceStatus,
    invoiceNumber: f.invoiceStatus === "not_invoiced" ? null : orNull(f.invoiceNumber),
    invoicedOn: f.invoiceStatus === "not_invoiced" ? null : orNull(f.invoicedOn),
    paidOn: f.invoiceStatus === "paid" ? orNull(f.paidOn) : null,
    ...(withMoney ? { money: moneyBody(f) } : {}),
  };
}
