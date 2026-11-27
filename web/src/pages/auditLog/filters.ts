// Settings › Audit log: the filters (who, what, from, to) and the open tab,
// kept in the page address ("#/settings/audit-log?tab=salary&who=usr_…"),
// so a link reopens the same view. "What" is a kind of record on the Changes
// tab (an action prefix the API filters on) and a kind of salary view on the
// Salary views tab.
import type { Schemas } from "../../api/client";

export type Tab = "changes" | "salary";
export type Filters = { who: string; what: string; from: string; to: string };

/** The actor id of what HyPlug did on its own. */
export const AUTOMATIC = "system";
export const PAGE_SIZE = 50;
export const auditLogKey = ["audit-log"] as const;
export const salaryViewsKey = ["salary-views"] as const;

/**
 * The kinds of change in "What", each the actions (or kinds of action) the API finds: one, or several
 * separated by commas.
 */
export const CHANGE_KINDS: { key: string; label: string; action: string }[] = [
  { key: "candidates", label: "Candidates", action: "candidate" },
  { key: "jobs", label: "Jobs", action: "job" },
  { key: "companies", label: "Companies", action: "company" },
  { key: "contacts", label: "Contacts", action: "contact" },
  { key: "pipeline", label: "Pipeline", action: "application" },
  { key: "interviews", label: "Interviews", action: "interview" },
  { key: "offers", label: "Offers", action: "offer" },
  { key: "placements", label: "Placements", action: "placement" },
  { key: "emails", label: "Emails", action: "email" },
  { key: "documents", label: "Documents and CVs", action: "document,cv,cv_template,cv_import" },
  { key: "notes", label: "Notes", action: "note" },
  { key: "sign-ins", label: "Sign-ins", action: "user.signed_in" },
  { key: "settings", label: "Settings", action: "settings,invoice_settings,tenant.renamed,ai_usage.settings_saved,aiprovider,reports.settings_saved,agency.two_step_required" },
  { key: "roles", label: "Roles and access", action: "role" },
];

export type SalaryKind = Schemas["SalaryViewKind"];

/** What the person was shown, in words. */
export const SALARY_KIND: Record<SalaryKind, string> = {
  candidate: "Opened the candidate",
  candidate_list: "Candidate list with salary",
  export: "Exported with salary",
  email_fields: "Writing an email with salary",
  email_sent: "Sent an email with salary",
  email_read: "Read an email with salary",
  cv: "Formatted CV with salary",
  screening: "Screening call salary",
  offer: "Offer salary",
  placement: "Placement salary",
};

const isSalaryKind = (k: string): k is SalaryKind => k in SALARY_KIND;
const isDay = (d: string) => /^\d{4}-\d{2}-\d{2}$/.test(d);

/** A date as YYYY-MM-DD, in the viewer's time zone. */
export function localDay(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${String(d.getFullYear())}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** The last 30 days, today included. */
export function defaultFilters(now = new Date()): Filters {
  const from = new Date(now);
  from.setDate(from.getDate() - 29);
  return { who: "", what: "", from: localDay(from), to: localDay(now) };
}

/**
 * The filters on switching to a tab: "What" means another thing on each, and nobody is shown a salary
 * automatically (Salary views has no "Automatic" in Who).
 */
export function filtersFor(tab: Tab, f: Filters): Filters {
  return { ...f, what: "", who: tab === "salary" && f.who === AUTOMATIC ? "" : f.who };
}

/** The tab and filters an address names; anything missing or not understood takes its default. */
export function readView(hash: string, canSalary: boolean, now = new Date()): { tab: Tab; filters: Filters } {
  const at = hash.indexOf("?");
  const q = new URLSearchParams(at < 0 ? "" : hash.slice(at + 1));
  const tab: Tab = canSalary && q.get("tab") === "salary" ? "salary" : "changes";
  const d = defaultFilters(now);
  const what = q.get("what") ?? "";
  const known = tab === "salary" ? isSalaryKind(what) : CHANGE_KINDS.some((k) => k.key === what);
  const day = (name: "from" | "to") => {
    const v = q.get(name);
    return v === null ? d[name] : v === "" || isDay(v) ? v : d[name]; // "from=" is "any day"
  };
  const who = q.get("who") ?? "";
  return { tab, filters: { who: tab === "salary" && who === AUTOMATIC ? "" : who, what: known ? what : "", from: day("from"), to: day("to") } };
}

/** The address of a view (dates always written, so a link shows the same days later). */
export function viewHash(tab: Tab, f: Filters): string {
  const q = new URLSearchParams();
  if (tab === "salary") q.set("tab", "salary");
  if (f.who) q.set("who", f.who);
  if (f.what) q.set("what", f.what);
  q.set("from", f.from);
  q.set("to", f.to);
  return `#/settings/audit-log?${q.toString()}`;
}

type Days = { from?: string; to?: string };

/**
 * The days sent. "To" left at today (its default) is sent as no end at all: the API counts days in
 * the agency's time zone, and the viewer's today may end before the agency's, which would leave out
 * what happened since.
 */
const days = (f: Filters, now: Date): Days => ({
  ...(f.from ? { from: f.from } : {}),
  ...(f.to && f.to !== localDay(now) ? { to: f.to } : {}),
});

/** The audit log query (and export body) for the filters. */
export function changesQuery(f: Filters, now = new Date()): Days & { actor?: string; action?: string } {
  const action = CHANGE_KINDS.find((k) => k.key === f.what)?.action;
  return { ...(f.who ? { actor: f.who } : {}), ...(action ? { action } : {}), ...days(f, now) };
}

/** The salary views query (and export body) for the filters. */
export function salaryQuery(f: Filters, now = new Date()): Days & { user?: string; kind?: SalaryKind } {
  return { ...(f.who ? { user: f.who } : {}), ...(isSalaryKind(f.what) ? { kind: f.what } : {}), ...days(f, now) };
}
