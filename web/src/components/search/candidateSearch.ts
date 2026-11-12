// An advanced candidate search as the page keeps it, and as the API takes it.
import type { Schemas } from "../../api/client";
import { countryName, formatNotice } from "../../labels";
import { AVAILABILITY } from "../candidateLook";

type Availability = Schemas["CandidateAvailability"];
type Kind = Schemas["WorkAuthorizationKind"];

export type SearchState = {
  boolean: string;
  availability: Availability[];
  workAuthorization: Kind[];
  nationality: string[];
  residence: string[];
  source: string[];
  industry: string[];
  skills: string[];
  skillsMatch: "all" | "any";
  tags: string[];
  location: string;
  noticeMaxDays: number | null;
  salaryMax: string; // RM per month
  updatedWithinDays: number | null;
};

export const EMPTY: SearchState = {
  boolean: "", availability: [], workAuthorization: [], nationality: [], residence: [], source: [], industry: [], skills: [],
  skillsMatch: "all", tags: [], location: "", noticeMaxDays: null, salaryMax: "", updatedWithinDays: null,
};

export const WORK_KINDS: { value: Kind; label: string }[] = [
  { value: "local", label: "Local" }, { value: "pr", label: "PR" }, { value: "expat", label: "Spousal / RP-T / other pass" },
  { value: "needs_pass", label: "Needs pass" }, { value: "unknown", label: "Not known" },
];

export const NOTICE_CHOICES: { days: number; label: string }[] = [
  { days: 0, label: "Immediate" }, { days: 14, label: "2 weeks or less" }, { days: 30, label: "1 month or less" },
  { days: 60, label: "2 months or less" }, { days: 90, label: "3 months or less" },
];

export const UPDATED_CHOICES: { days: number; label: string }[] = [
  { days: 7, label: "Past week" }, { days: 30, label: "Past month" }, { days: 90, label: "Past 3 months" }, { days: 365, label: "Past year" },
];

export function isEmpty(s: SearchState): boolean {
  return JSON.stringify({ ...s, skillsMatch: "all" }) === JSON.stringify(EMPTY);
}

export type CandidateSort = Schemas["CandidateSort"];
/** Where a page starts: an id cursor (newest first) or an offset (other sorts). */
export type PageAt = { cursor?: string; offset?: number };
/** The next page of a list or search, from the server's answer. */
export const nextPage = (last: { nextCursor?: string | null; nextOffset?: number | null }): PageAt | undefined =>
  last.nextCursor ? { cursor: last.nextCursor } : last.nextOffset != null ? { offset: last.nextOffset } : undefined;

/**
 * Columns whose header sorts the list, like a spreadsheet: the first click
 * sorts A to Z, shortest or lowest first; the next click flips it.
 */
export const SORTABLE: Record<string, { asc: CandidateSort; desc: CandidateSort; salary?: boolean }> = {
  name: { asc: "name", desc: "name_desc" },
  noticePeriod: { asc: "notice", desc: "notice_desc" },
  currentSalary: { asc: "current_salary_asc", desc: "current_salary_desc", salary: true },
  expectedSalary: { asc: "expected_salary_asc", desc: "expected_salary_desc", salary: true },
};

/** The next order when a header is clicked: its first order, or the other one if it is sorted already. */
export function flipSort(current: CandidateSort, column: string): CandidateSort {
  const c = SORTABLE[column];
  if (!c) return current;
  return current === c.asc ? c.desc : c.asc;
}

/** Whether (and which way) a column's header shows the list sorted. */
export function sortOf(current: CandidateSort, column: string): "ascending" | "descending" | undefined {
  const c = SORTABLE[column];
  return !c ? undefined : current === c.asc ? "ascending" : current === c.desc ? "descending" : undefined;
}

/** The request for a page of the search (q and "mine" come from the page's own boxes). */
export function toBody(s: SearchState, q: string, mine: boolean, page?: PageAt, sort: CandidateSort = "newest"): Schemas["CandidateSearch"] {
  const body: Schemas["CandidateSearch"] = { limit: 25, skillsMatch: s.skillsMatch };
  const list = <T extends string>(v: T[]) => (v.length > 0 ? v : undefined);
  if (q.trim()) body.q = q.trim();
  if (s.boolean.trim()) body.boolean = s.boolean.trim();
  body.availability = list(s.availability);
  body.workAuthorization = list(s.workAuthorization);
  body.nationality = list(s.nationality);
  body.residence = list(s.residence);
  body.source = list(s.source);
  body.industry = list(s.industry);
  body.skills = list(s.skills);
  body.tags = list(s.tags);
  if (s.location.trim()) body.location = s.location.trim();
  if (s.noticeMaxDays !== null) body.noticeMaxDays = s.noticeMaxDays;
  const salary = s.salaryMax.replace(/[^0-9.]/g, "");
  if (salary) body.expectedSalaryMax = { amount: salary, currency: "MYR" };
  if (s.updatedWithinDays !== null) body.updatedWithinDays = s.updatedWithinDays;
  if (mine) body.owner = "me";
  if (page?.cursor) body.cursor = page.cursor;
  if (page?.offset) body.offset = page.offset;
  if (sort !== "newest") body.sort = sort;
  return JSON.parse(JSON.stringify(body)) as Schemas["CandidateSearch"]; // without the undefined ones
}

/** What is set, as removable chips: "Status: Looking out". */
export function activeChips(s: SearchState, sourceLabel: (key: string) => string, industryLabel: (key: string) => string = (key) => key): { label: string; clear: (s: SearchState) => SearchState }[] {
  const out: { label: string; clear: (s: SearchState) => SearchState }[] = [];
  const many = (name: string, values: string[], show: (v: string) => string, clear: (s: SearchState) => SearchState) => {
    if (values.length > 0) out.push({ label: `${name}: ${values.map(show).join(", ")}`, clear });
  };
  if (s.boolean.trim()) out.push({ label: `Boolean: ${s.boolean.trim()}`, clear: (x) => ({ ...x, boolean: "" }) });
  many("Status", s.availability, (v) => AVAILABILITY[v as Availability].label, (x) => ({ ...x, availability: [] }));
  many("Work authorization", s.workAuthorization, (v) => WORK_KINDS.find((k) => k.value === v)?.label ?? v, (x) => ({ ...x, workAuthorization: [] }));
  many("Nationality", s.nationality, countryName, (x) => ({ ...x, nationality: [] }));
  many("Lives in", s.residence, countryName, (x) => ({ ...x, residence: [] }));
  many("Source", s.source, sourceLabel, (x) => ({ ...x, source: [] }));
  many("Industry", s.industry, industryLabel, (x) => ({ ...x, industry: [] }));
  many(s.skillsMatch === "any" && s.skills.length > 1 ? "Any skill of" : "Skills", s.skills, (v) => v, (x) => ({ ...x, skills: [] }));
  many("Tags", s.tags, (v) => v, (x) => ({ ...x, tags: [] }));
  if (s.location.trim()) out.push({ label: `Location: ${s.location.trim()}`, clear: (x) => ({ ...x, location: "" }) });
  if (s.noticeMaxDays !== null) {
    out.push({ label: `Notice: ${NOTICE_CHOICES.find((n) => n.days === s.noticeMaxDays)?.label ?? formatNotice({ value: s.noticeMaxDays, unit: "day" })}`, clear: (x) => ({ ...x, noticeMaxDays: null }) });
  }
  if (s.salaryMax.trim()) out.push({ label: `Expects up to RM ${s.salaryMax.trim()} / month`, clear: (x) => ({ ...x, salaryMax: "" }) });
  if (s.updatedWithinDays !== null) {
    out.push({ label: `Updated: ${UPDATED_CHOICES.find((u) => u.days === s.updatedWithinDays)?.label ?? ""}`, clear: (x) => ({ ...x, updatedWithinDays: null }) });
  }
  return out;
}

const KEY = "hiringplug.candidateSearch";

/** The search kept for this browser tab, so coming back from a candidate keeps it. */
export function loadSearch(): SearchState {
  try {
    const raw = sessionStorage.getItem(KEY);
    return raw ? { ...EMPTY, ...(JSON.parse(raw) as Partial<SearchState>) } : EMPTY;
  } catch {
    return EMPTY;
  }
}

export function saveSearch(s: SearchState): void {
  try {
    if (isEmpty(s)) sessionStorage.removeItem(KEY);
    else sessionStorage.setItem(KEY, JSON.stringify(s));
  } catch { /* private window: not kept */ }
}

/** A snippet's text and its marked words ("led [[SAP]] upgrades"). */
export function snippetParts(snippet: string): { text: string; match: boolean }[] {
  return snippet.split(/(\[\[.*?\]\])/).filter(Boolean)
    .map((p) => (p.startsWith("[[") && p.endsWith("]]") ? { text: p.slice(2, -2), match: true } : { text: p, match: false }));
}

/** A boolean as coloured tokens, as it will be read: operators, phrases, words left out. */
export function booleanTokens(input: string): { text: string; kind: "op" | "not" | "phrase" | "group" | "word" | "space" }[] {
  const out: { text: string; kind: "op" | "not" | "phrase" | "group" | "word" | "space" }[] = [];
  const re = /(\s+)|("[^"]*"?)|([()])|([^\s()"]+)/g;
  let m: RegExpExecArray | null;
  let negate = false;
  while ((m = re.exec(input)) !== null) {
    const [tok, space, phrase, group] = m;
    if (space) { out.push({ text: tok, kind: "space" }); continue; }
    if (group) { out.push({ text: tok, kind: "group" }); negate = false; continue; }
    const upper = tok.toUpperCase();
    if (upper === "AND" || upper === "OR") { out.push({ text: tok, kind: "op" }); continue; }
    if (upper === "NOT") { out.push({ text: tok, kind: "not" }); negate = true; continue; }
    if (tok.startsWith("-") && tok.length > 1) { out.push({ text: tok, kind: "not" }); continue; }
    out.push({ text: tok, kind: negate ? "not" : phrase ? "phrase" : "word" });
    negate = false;
  }
  return out;
}
