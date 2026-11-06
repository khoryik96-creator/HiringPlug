// The candidate page's colours: the banner by status (availability), the
// column on its right by work authorization; and the figures it shows.
import type { Candidate, Compensation, Schemas } from "../api/client";
import { PAY_PERIOD } from "../labels";

export type Availability = Schemas["CandidateAvailability"];
type WorkAuth = Schemas["WorkAuthorization"];

export const AVAILABILITY: Record<Availability, { label: string; bg: string; edge: string }> = {
  looking: { label: "Looking out", bg: "#FFF3B0", edge: "#C9A227" },
  passive: { label: "Passive", bg: "#CDEFC8", edge: "#4E9A47" },
  placed: { label: "Placed", bg: "#E0D4F7", edge: "#7C5BC4" },
  not_looking: { label: "Not looking", bg: "#F9CFDF", edge: "#C2456F" },
};
export const NO_STATUS_BG = "#EEF1F5";

/** The work authorization choices on the page (each a colour), from the stored status and pass. */
export type WaKind = "local" | "pr" | "expat" | "needsPass" | "unknown";

export const WA_KIND: Record<WaKind, { label: string; color: string }> = {
  local: { label: "Local", color: "#15803D" },
  pr: { label: "PR", color: "#7E22CE" },
  expat: { label: "Spousal / RP-T", color: "#1D4ED8" },
  needsPass: { label: "Needs pass", color: "#B91C1C" },
  unknown: { label: "Not known", color: "#6B7280" },
};

export function waKind(wa: WorkAuth | null | undefined): WaKind {
  if (!wa) return "unknown";
  if (wa.status === "local") return "local";
  if (wa.status === "requires_visa") return "needsPass";
  return wa.basis === "permanent_resident" ? "pr" : "expat";
}

/** What the column says: the kind, or for an expat the pass they hold. */
export function waColumnLabel(wa: WorkAuth | null | undefined): string {
  const kind = waKind(wa);
  if (kind !== "expat") return WA_KIND[kind].label;
  if (wa?.basis === "rp_t") return "RP-T";
  if (wa?.basis === "spouse_visa") return "Spouse visa";
  return "Expat";
}

/** The work authorization to save for a choice; an expat needs the pass held. Any note is kept. */
export function waFor(kind: WaKind, current: WorkAuth | null | undefined, pass?: "rp_t" | "spouse_visa"): WorkAuth | null {
  const note = current?.note ?? null;
  switch (kind) {
    case "local": return { status: "local", basis: null, note };
    case "pr": return { status: "expat_eligible", basis: "permanent_resident", note };
    case "expat": return { status: "expat_eligible", basis: pass ?? "rp_t", note };
    case "needsPass": return { status: "requires_visa", basis: null, note };
    case "unknown": return null;
  }
}

type Salary = NonNullable<Compensation["currentSalary"]>;
type Range = NonNullable<Compensation["expectedSalary"]>;

const money = (currency: string, amount: string) => `${currency === "MYR" ? "RM" : currency} ${Number(amount).toLocaleString("en-MY", { maximumFractionDigits: 2 })}`;

export function salaryText(s: Salary): [string, string] {
  return [money(s.currency, s.amount), `/ ${PAY_PERIOD[s.period]}`];
}

export function rangeText(r: Range): [string, string] {
  const amount = r.max ? `${money(r.currency, r.min)} – ${Number(r.max).toLocaleString("en-MY")}` : money(r.currency, r.min);
  return [amount, `/ ${PAY_PERIOD[r.period]}`];
}

/** "+26% on current", when both figures are in the same currency and period. */
export function raiseText(c: Compensation | null | undefined): string | null {
  const cur = c?.currentSalary;
  const exp = c?.expectedSalary;
  if (!cur || !exp || cur.currency !== exp.currency || cur.period !== exp.period || Number(cur.amount) <= 0) return null;
  const pct = Math.round(((Number(exp.min) - Number(cur.amount)) / Number(cur.amount)) * 100);
  if (pct === 0) return "About the same as current";
  return `${pct > 0 ? "+" : ""}${String(pct)}% on current`;
}

/** Title and company as one line, whichever are known. */
export function jobLine(c: Candidate): string {
  return [c.currentTitle, c.currentCompany].filter(Boolean).join(" · ");
}
