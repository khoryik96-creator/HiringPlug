// The records you have open (candidates, contacts, companies, jobs), kept
// as tabs in this browser: each named after what it is (the candidate's or
// contact's name, the company, the job title), so you can go back and forth.
import { useEffect, useSyncExternalStore } from "react";

export type TabKind = "candidate" | "contact" | "company" | "job";
/** done: something finished on it (a CV formatted) since you last had it open. */
export type OpenTab = { href: string; kind: TabKind; label: string; done?: boolean };

/** At most this many tabs: opening another closes the one least recently opened. */
export const MAX_TABS = 30;
const KEY = "hp.openTabs";
const ROUTES: [RegExp, TabKind, string][] = [
  [/^#\/candidates\/(cand_[0-9a-z]+)(?:\/(?:screening|reference-check|compare))?(?:\?.*)?$/, "candidate", "#/candidates/"],
  [/^#\/contacts\/(cont_[0-9a-z]+)(?:\?.*)?$/, "contact", "#/contacts/"],
  [/^#\/companies\/(comp_[0-9a-z]+)(?:\?.*)?$/, "company", "#/companies/"],
  [/^#\/jobs\/(jo_[0-9a-z]+)(?:\?.*)?$/, "job", "#/jobs/"],
];

/** The record a page's address is about (its tab), or null. Pure: unit tested. */
export function recordOf(hash: string): { href: string; kind: TabKind } | null {
  for (const [re, kind, prefix] of ROUTES) {
    const m = re.exec(hash);
    if (m?.[1]) return { href: prefix + m[1], kind };
  }
  return null;
}

function load(): OpenTab[] {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? "[]") as unknown;
    if (!Array.isArray(saved)) return [];
    return saved.filter((t): t is OpenTab => typeof t === "object" && t !== null && typeof (t as OpenTab).href === "string"
      && recordOf((t as OpenTab).href)?.href === (t as OpenTab).href && typeof (t as OpenTab).label === "string").slice(-MAX_TABS);
  } catch { return []; }
}

let tabs: OpenTab[] = load();
const listeners = new Set<() => void>();
function set(next: OpenTab[]) {
  tabs = next;
  try { localStorage.setItem(KEY, JSON.stringify(tabs)); } catch { /* private window: kept for this visit only */ }
  listeners.forEach((l) => { l(); });
}

/** Opens a record's tab (at the end), or keeps it where it is if open already. */
export function openTab(href: string, kind: TabKind) {
  if (tabs.some((t) => t.href === href)) return;
  set([...tabs, { href, kind, label: "" }].slice(-MAX_TABS));
}

/** Names a record's tab once its page knows the name. */
export function nameTab(href: string, label: string) {
  const name = label.trim();
  if (!name || !tabs.some((t) => t.href === href && t.label !== name)) return;
  set(tabs.map((t) => (t.href === href ? { ...t, label: name } : t)));
}

export function closeTabs(hrefs: string[]) {
  set(tabs.filter((t) => !hrefs.includes(t.href)));
}

export function closeTab(href: string) { closeTabs([href]); }

// What has lit a tab already (a notification's id): never twice.
const LIT_KEY = "hp.openTabsLit";
let lit: string[] = (() => {
  try { const v = JSON.parse(localStorage.getItem(LIT_KEY) ?? "[]") as unknown; return Array.isArray(v) ? v.filter((x) => typeof x === "string") : []; } catch { return []; }
})();

function remember(event: string) {
  lit = [...lit, event].slice(-200);
  try { localStorage.setItem(LIT_KEY, JSON.stringify(lit)); } catch { /* private window */ }
}

/** Lights a record's tab green: something finished on it (once per event).
 * On the record's own page (looking) it is seen there and never lights;
 * with no tab open for it, it waits until one is. */
export function markTabDone(href: string, event: string, looking = false) {
  if (lit.includes(event)) return;
  if (looking) { remember(event); return; }
  if (!tabs.some((t) => t.href === href)) return;
  remember(event);
  set(tabs.map((t) => (t.href === href ? { ...t, done: true } : t)));
}

/** The record is open again: its tab is no longer lit. */
export function seenTab(href: string) {
  if (tabs.some((t) => t.href === href && t.done)) set(tabs.map((t) => (t.href === href ? { ...t, done: false } : t)));
}

export function useOpenTabs(): OpenTab[] {
  return useSyncExternalStore((l) => { listeners.add(l); return () => { listeners.delete(l); }; }, () => tabs);
}

/** A record page names its tab: the candidate's name, the job title. */
export function useTabName(href: string, label: string | undefined) {
  useEffect(() => { if (label) nameTab(href, label); }, [href, label]);
}

/** For tests: start with no tabs. */
export function resetTabs() { lit = []; set([]); }
