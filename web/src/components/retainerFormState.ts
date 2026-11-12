// A job's retainer in the form: rows of stages, and the body sent (pure, unit tested).
import type { Schemas } from "../api/client";
import { plainAmount } from "./salaryFormState";

type Retainer = Schemas["JobRetainer"];
type DueKind = Schemas["RetainerDueKind"];

export type StageRow = {
  label: string;
  amount: string;
  dueKind: DueKind;
  dueOn: string; // on_date
  dueDays: string; // after_signing, after_start
  invoiceStatus: Schemas["InvoiceStatus"];
  invoiceNumber: string;
  invoicedOn: string;
  paidOn: string;
};

export type RetainerForm = { currency: string; signedOn: string; stages: StageRow[] };

export const DUE_KIND: Record<DueKind, string> = {
  on_date: "On a date",
  after_signing: "Days after signing",
  after_start: "Days after the start",
};

export const blankStage = (label = ""): StageRow => ({
  label, amount: "", dueKind: "on_date", dueOn: "", dueDays: "", invoiceStatus: "not_invoiced", invoiceNumber: "", invoicedOn: "", paidOn: "",
});

/** A retainer as the form shows it; none yet: the usual three stages to fill in. */
export function retainerForm(r: Retainer | undefined, currency = "MYR"): RetainerForm {
  if (!r || r.stages.length === 0) {
    return { currency, signedOn: "", stages: ["Engagement", "Shortlist", "Completion"].map((l) => blankStage(l)) };
  }
  return {
    currency: r.currency ?? currency,
    signedOn: r.signedOn ?? "",
    stages: r.stages.map((s) => ({
      label: s.label, amount: s.amount, dueKind: s.dueKind,
      dueOn: s.dueKind === "on_date" ? (s.dueOn ?? "") : "", dueDays: s.dueDays === null ? "" : String(s.dueDays),
      invoiceStatus: s.invoiceStatus, invoiceNumber: s.invoiceNumber ?? "", invoicedOn: s.invoicedOn ?? "", paidOn: s.paidOn ?? "",
    })),
  };
}

const orUndefined = (s: string) => (s.trim() === "" ? undefined : s.trim());

/** What is sent: stages without a name or amount are left out (all empty: no retainer). */
export function retainerBody(f: RetainerForm, version: number): Schemas["JobRetainerInput"] {
  const stages = f.stages.filter((s) => s.label.trim() !== "" || plainAmount(s.amount) !== "").map((s): Schemas["RetainerStageInput"] => ({
    label: s.label.trim(),
    amount: plainAmount(s.amount),
    dueKind: s.dueKind,
    ...(s.dueKind === "on_date" ? { dueOn: orUndefined(s.dueOn) } : { dueDays: s.dueDays.trim() === "" ? undefined : Number(s.dueDays) }),
    invoiceStatus: s.invoiceStatus,
    ...(s.invoiceStatus !== "not_invoiced" ? { invoiceNumber: orUndefined(s.invoiceNumber), invoicedOn: orUndefined(s.invoicedOn) } : {}),
    ...(s.invoiceStatus === "paid" ? { paidOn: orUndefined(s.paidOn) } : {}),
  }));
  return { version, currency: f.currency.trim().toUpperCase(), signedOn: f.signedOn || null, stages };
}

/** Problems the form can see before sending, keyed like the server's (stages.0.amount). */
export function retainerProblems(body: Schemas["JobRetainerInput"]): Record<string, string> {
  const out: Record<string, string> = {};
  if (body.stages.length > 0 && !/^[A-Z]{3}$/.test(body.currency ?? "")) out.currency = "Use the 3-letter currency code, e.g. MYR.";
  body.stages.forEach((s, i) => {
    if (!s.label) out[`stages.${String(i)}.label`] = "Enter a name.";
    if (!/^(0|[1-9]\d*)(\.\d{1,2})?$/.test(s.amount) || Number(s.amount) <= 0) out[`stages.${String(i)}.amount`] = "Enter an amount like 15000 or 15,000.";
    if (s.dueKind === "on_date" && !s.dueOn) out[`stages.${String(i)}.dueOn`] = "Enter the day.";
    if (s.dueKind !== "on_date" && (s.dueDays === undefined || !Number.isInteger(s.dueDays) || s.dueDays < 0)) out[`stages.${String(i)}.dueDays`] = "Enter a number of days.";
  });
  return out;
}

/** "15 days after signing", "on 15 Dec 2026" (the day when known). */
export function dueText(s: Schemas["RetainerStage"], formatDay: (d: string) => string): string {
  if (s.dueKind === "on_date") return s.dueOn ? formatDay(s.dueOn) : "";
  const days = s.dueDays ?? 0;
  const what = s.dueKind === "after_signing" ? "signing" : "the start";
  const rule = days === 0 ? `On ${what}` : `${String(days)} days after ${what}`;
  return s.dueOn ? `${formatDay(s.dueOn)} · ${rule.toLowerCase()}` : rule;
}
