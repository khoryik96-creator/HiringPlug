// The placements dashboard's rules (pure, unit tested): where each
// placement's guarantee stands, the cards that count and filter them, the
// date range, and the filters and sorting of the table. "Today" is the
// agency's (the list says it), never the browser's clock.
import type { Schemas } from "../api/client";
import { formatDay } from "../labels";

type Placement = Schemas["Placement"];

/** Whole days from one day (YYYY-MM-DD) to another; negative when it is before. */
export function daysBetween(from: string, to: string): number {
  return Math.round((Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / 86_400_000);
}

/** The same day, n days later. */
export function addDays(day: string, n: number): string {
  return new Date(Date.parse(`${day}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);
}

/** A guarantee ending this many days from today or fewer is "ending soon". */
export const ENDING_SOON_DAYS = 30;

export type GuaranteeState = "not_started" | "in_guarantee" | "ending_soon" | "ended" | "fell_through" | "cancelled";
export const GUARANTEE_LABEL: Record<GuaranteeState, string> = {
  not_started: "Not started", in_guarantee: "In guarantee", ending_soon: "Ending soon",
  ended: "Guarantee over", fell_through: "Fell through", cancelled: "Did not start",
};

/** Where a placement's guarantee stands today, and a short line for the table. */
export function guarantee(p: Placement, today: string): { state: GuaranteeState; note: string } {
  if (p.status === "fell_through") return { state: "fell_through", note: p.leftOn ? `left ${formatDay(p.leftOn)}` : "" };
  if (p.status === "cancelled") return { state: "cancelled", note: "" };
  const toStart = daysBetween(today, p.startDate);
  if (toStart > 0) return { state: "not_started", note: toStart === 1 ? "starts tomorrow" : `starts in ${String(toStart)} days` };
  const left = daysBetween(today, p.guaranteeEndsOn);
  if (left < 0) return { state: "ended", note: "" };
  const note = left === 0 ? "ends today" : left === 1 ? "1 day left" : `${String(left)} days left`;
  return { state: left <= ENDING_SOON_DAYS ? "ending_soon" : "in_guarantee", note };
}

/** The invoice was sent and its due day has passed without payment. */
export const overdue = (p: Placement, today: string) => p.invoiceStatus === "invoiced" && p.invoicing.dueOn < today;

export type Kpi = "all" | "starting" | "in_guarantee" | "ending_soon" | "not_invoiced" | "overdue" | "replacement";
export const KPIS: { key: Kpi; label: string; hint: string }[] = [
  { key: "all", label: "Placements", hint: "Started in the dates chosen" },
  { key: "starting", label: "Starting soon", hint: "Not started yet" },
  { key: "in_guarantee", label: "In guarantee", hint: "Started, guarantee running" },
  { key: "ending_soon", label: "Guarantee ending", hint: `Within ${String(ENDING_SOON_DAYS)} days` },
  { key: "not_invoiced", label: "Not invoiced", hint: "Placed, no invoice yet" },
  { key: "overdue", label: "Overdue", hint: "Invoiced, past the due date" },
  { key: "replacement", label: "Replacement owed", hint: "Fell through in the guarantee" },
];

/** Whether a placement counts on a card. */
export function onCard(p: Placement, kpi: Kpi, today: string): boolean {
  switch (kpi) {
    case "all": return true;
    case "starting": return guarantee(p, today).state === "not_started";
    case "in_guarantee": { const s = guarantee(p, today).state; return s === "in_guarantee" || s === "ending_soon"; }
    case "ending_soon": return guarantee(p, today).state === "ending_soon";
    case "not_invoiced": return p.status !== "cancelled" && p.invoiceStatus === "not_invoiced";
    case "overdue": return overdue(p, today);
    case "replacement": return p.replacement === "owed";
  }
}

export type Range = "this_year" | "last_year" | "all" | "custom";
export type Sort = "start_desc" | "start_asc" | "guarantee_end" | "fee_desc" | "candidate" | "company";
export const SORTS: [Sort, string][] = [
  ["start_desc", "Newest start first"], ["start_asc", "Oldest start first"], ["guarantee_end", "Guarantee ending first"],
  ["fee_desc", "Highest fee first"], ["candidate", "Candidate A–Z"], ["company", "Company A–Z"],
];

/** What the dashboard shows: the dates, the card, the search, the filters and the order. */
export type View = {
  range: Range; from: string; to: string; kpi: Kpi; q: string;
  recruiter: string; company: string; outcome: Schemas["PlacementStatus"] | ""; invoice: "" | "not_invoiced" | "invoiced" | "overdue" | "paid";
  guarantee: GuaranteeState | ""; sort: Sort;
};
export const DEFAULT_VIEW: View = {
  range: "this_year", from: "", to: "", kpi: "all", q: "", recruiter: "", company: "", outcome: "", invoice: "", guarantee: "", sort: "start_desc",
};

/** The start dates to ask the server for (inclusive); empty for all time. */
export function rangeDates(v: View, today: string): { from: string; to: string } {
  const year = Number(today.slice(0, 4));
  if (v.range === "this_year") return { from: `${String(year)}-01-01`, to: `${String(year)}-12-31` };
  if (v.range === "last_year") return { from: `${String(year - 1)}-01-01`, to: `${String(year - 1)}-12-31` };
  if (v.range === "custom") return { from: v.from, to: v.to };
  return { from: "", to: "" };
}

const fee = (p: Placement) => Number(p.money?.feeAmount ?? Number.NaN);

/** The placements matching the search and filters (before the card), in order. */
export function filtered(items: Placement[], v: View, today: string): Placement[] {
  const words = v.q.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const out = items.filter((p) => {
    if (v.recruiter && p.recruiterId !== v.recruiter) return false; // the placement's own recruiter, never the job's owner
    if (v.company && p.companyId !== v.company) return false;
    if (v.outcome && p.status !== v.outcome) return false;
    if (v.invoice === "overdue" ? !overdue(p, today) : v.invoice === "invoiced" ? p.invoiceStatus !== "invoiced" || overdue(p, today) : v.invoice && p.invoiceStatus !== v.invoice) return false;
    if (v.guarantee && guarantee(p, today).state !== v.guarantee) return false;
    if (words.length) {
      const text = [p.candidateName, p.companyName, p.jobTitle, p.recruiterName ?? "", p.poNumber ?? "", p.invoiceNumber ?? ""].join(" ").toLowerCase();
      if (!words.every((w) => text.includes(w))) return false;
    }
    return true;
  });
  const by: Record<Sort, (a: Placement, b: Placement) => number> = {
    start_desc: (a, b) => b.startDate.localeCompare(a.startDate),
    start_asc: (a, b) => a.startDate.localeCompare(b.startDate),
    guarantee_end: (a, b) => a.guaranteeEndsOn.localeCompare(b.guaranteeEndsOn),
    fee_desc: (a, b) => (Number.isNaN(fee(b)) ? -1 : fee(b)) - (Number.isNaN(fee(a)) ? -1 : fee(a)),
    candidate: (a, b) => a.candidateName.localeCompare(b.candidateName),
    company: (a, b) => a.companyName.localeCompare(b.companyName) || b.startDate.localeCompare(a.startDate),
  };
  return out.sort((a, b) => by[v.sort](a, b) || b.startDate.localeCompare(a.startDate));
}

/** The guarantee view: those ending soonest first, within the card's filter. */
export const forCard = (items: Placement[], kpi: Kpi, today: string) => items.filter((p) => onCard(p, kpi, today));

const VIEW_KEY = "hiringplug.placements.view";

/** The view saved in this browser (dates, card, filters, order). */
export function loadView(): View {
  try {
    const saved = JSON.parse(localStorage.getItem(VIEW_KEY) ?? "{}") as Partial<View>;
    const v = { ...DEFAULT_VIEW, ...saved };
    return {
      ...v,
      range: (["this_year", "last_year", "all", "custom"] as Range[]).includes(v.range) ? v.range : "this_year",
      kpi: KPIS.some((k) => k.key === v.kpi) ? v.kpi : "all",
      sort: SORTS.some(([k]) => k === v.sort) ? v.sort : "start_desc",
    };
  } catch { return DEFAULT_VIEW; }
}

export function saveView(v: View) {
  try { localStorage.setItem(VIEW_KEY, JSON.stringify(v)); } catch { /* private window: not remembered */ }
}
