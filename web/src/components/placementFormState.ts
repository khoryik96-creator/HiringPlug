// The placement form's state and the request bodies built from it (pure, unit tested).
import type { Schemas } from "../api/client";

type Placement = Schemas["Placement"];

export type PlacementFormState = {
  // The sheet: title, client and job type start from the job ("" job type:
  // not known yet, so the server takes it from the job).
  jobTitle: string;
  companyName: string;
  jobType: Schemas["PlacementJobType"] | "";
  /** The title and client as the form opened with them: unchanged, they are not sent (a cleared one keeps following the job). */
  loadedNames: { jobTitle: string; companyName: string };
  poNumber: string;
  endDate: string;
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
  // Invoicing: who receives it, the terms, a due date someone set ("": the
  // invoice date plus the terms), and emailing it on the start date.
  contactId: string;
  paymentTermsDays: string;
  dueOn: string;
  autoSend: boolean;
};

export function formFromPlacement(p: Placement | undefined, recruiterId: string): PlacementFormState {
  const money = p?.money ?? null;
  return {
    jobTitle: p?.jobTitle ?? "",
    companyName: p?.companyName ?? "",
    jobType: p?.jobType ?? "",
    loadedNames: { jobTitle: p?.jobTitle ?? "", companyName: p?.companyName ?? "" },
    poNumber: p?.poNumber ?? "",
    endDate: p?.endDate ?? "",
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
    contactId: p?.invoicing.contactId ?? "",
    paymentTermsDays: String(p?.invoicing.paymentTermsDays ?? 30),
    dueOn: p?.invoicing.dueOnSet ? p.invoicing.dueOn : "",
    autoSend: p?.invoicing.autoSend ?? false,
  };
}

/** A new placement's form with the client's terms of business and hiring contact (what the user has not typed yet). */
export function withDefaults(f: PlacementFormState, d: Schemas["PlacementDefaults"], withMoney: boolean): PlacementFormState {
  return {
    ...f,
    jobTitle: f.jobTitle || d.jobTitle,
    companyName: f.companyName || d.companyName,
    jobType: f.jobType || d.jobType,
    guaranteeDays: String(d.guaranteeDays),
    paymentTermsDays: String(d.paymentTermsDays),
    contactId: f.contactId || (d.contactId ?? ""),
    autoSend: f.contactId ? f.autoSend : d.contactId !== null,
    feePercent: withMoney && f.feeKind === "percent" && !f.feePercent && d.feePercent ? d.feePercent : f.feePercent,
  };
}

/** The due date when nobody sets one: the start date (the invoice date) plus the terms. */
export function autoDueOn(startDate: string, termsDays: string): string {
  const days = Number(termsDays);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(startDate) || !Number.isInteger(days)) return "";
  const d = new Date(`${startDate}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

export function invoicingBody(f: PlacementFormState): Schemas["PlacementInvoicingInput"] {
  return { contactId: orNull(f.contactId), paymentTermsDays: Number(f.paymentTermsDays), dueOn: orNull(f.dueOn), autoSend: f.autoSend };
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
    ...(f.jobType ? { jobType: f.jobType } : {}),
    ...(f.jobTitle.trim() ? { jobTitle: f.jobTitle.trim() } : {}),
    ...(f.companyName.trim() ? { companyName: f.companyName.trim() } : {}),
    ...(f.poNumber.trim() ? { poNumber: f.poNumber.trim() } : {}),
    ...(f.endDate ? { endDate: f.endDate } : {}),
    ...(f.recruiterId ? { recruiterId: f.recruiterId } : {}),
    ...(money ? { money } : {}),
  };
}

export function updateBody(f: PlacementFormState, version: number, withMoney: boolean): Schemas["PlacementUpdate"] {
  return {
    version,
    // Only what was changed: a cleared title or client name goes back to the job's.
    ...(f.jobTitle.trim() !== f.loadedNames.jobTitle ? { jobTitle: f.jobTitle.trim() } : {}),
    ...(f.companyName.trim() !== f.loadedNames.companyName ? { companyName: f.companyName.trim() } : {}),
    ...(f.jobType ? { jobType: f.jobType } : {}),
    poNumber: orNull(f.poNumber),
    endDate: orNull(f.endDate),
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
