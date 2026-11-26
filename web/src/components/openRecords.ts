// The records you have open (candidates, contacts, companies, jobs), kept
// as tabs in this browser: each named after what it is (the candidate's or
// contact's name, the company, the job title), so you can go back and forth.
// They are the signed-in user's own (kept per user and company, forgotten on
// signing out), and the same in every window of the browser.
import { useEffect, useSyncExternalStore } from "react";

export type TabKind = "candidate" | "contact" | "company" | "job";
/** done: something finished on it (a CV formatted) since you last had it open.
 * unseen: opened behind the page you were on, and not looked at yet. */
export type OpenTab = { href: string; kind: TabKind; label: string; done?: boolean; unseen?: boolean };

/** At most this many tabs: opening another closes the one least recently opened. */
export const MAX_TABS = 30;
/** Whose tabs these are ("tenant:user"): none until someone is signed in. */
let owner = "";
const key = () => `hp.openTabs:${owner}`;
const litKey = () => `hp.openTabsLit:${owner}`;
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
  if (!owner) return [];
  try {
    const saved = JSON.parse(localStorage.getItem(key()) ?? "[]") as unknown;
    if (!Array.isArray(saved)) return [];
    return saved.filter((t): t is OpenTab => typeof t === "object" && t !== null && typeof (t as OpenTab).href === "string"
      && recordOf((t as OpenTab).href)?.href === (t as OpenTab).href && typeof (t as OpenTab).label === "string").slice(-MAX_TABS);
  } catch { return []; }
}

function loadLit(): string[] {
  if (!owner) return [];
  try { const v = JSON.parse(localStorage.getItem(litKey()) ?? "[]") as unknown; return Array.isArray(v) ? v.filter((x) => typeof x === "string") : []; } catch { return []; }
}

let tabs: OpenTab[] = [];
// What has lit a tab already (a notification's id): never twice.
let lit: string[] = [];
const listeners = new Set<() => void>();
const notify = () => { listeners.forEach((l) => { l(); }); };
function set(next: OpenTab[]) {
  if (!owner) return; // nobody signed in: nothing kept
  tabs = next;
  try { localStorage.setItem(key(), JSON.stringify(tabs)); } catch { /* private window: kept for this visit only */ }
  notify();
}

/** The signed-in user's tabs: called as the app opens for them (no re-render needed: nothing is shown yet). */
export function setTabsOwner(who: string) {
  if (who === owner) return;
  owner = who;
  tabs = load();
  lit = loadLit();
}

/** Signing out: this user's tabs are forgotten (another person may use the browser next). */
export function forgetTabs() {
  if (owner) {
    try { localStorage.removeItem(key()); localStorage.removeItem(litKey()); } catch { /* private window */ }
  }
  owner = "";
  tabs = [];
  lit = [];
  notify();
}

/** Opens a record's tab (at the end), or keeps it where it is if open already.
 * behind: opened without going to it, so it is marked until it is looked at. */
export function openTab(href: string, kind: TabKind, behind = false) {
  if (tabs.some((t) => t.href === href)) return;
  set([...tabs, { href, kind, label: "", ...(behind && { unseen: true }) }].slice(-MAX_TABS));
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

function remember(event: string) {
  if (!owner) return;
  lit = [...lit, event].slice(-200);
  try { localStorage.setItem(litKey(), JSON.stringify(lit)); } catch { /* private window */ }
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

/** The record is open again: its tab is no longer lit, nor marked as not looked at. */
export function seenTab(href: string) {
  if (tabs.some((t) => t.href === href && (t.done || t.unseen))) set(tabs.map((t) => (t.href === href ? { ...t, done: false, unseen: false } : t)));
}

// Another window opened or closed a tab: these follow.
function fromOtherWindow(e: StorageEvent) {
  if (!owner || (e.key !== key() && e.key !== litKey())) return;
  tabs = load();
  lit = loadLit();
  notify();
}

export function useOpenTabs(): OpenTab[] {
  return useSyncExternalStore((l) => {
    listeners.add(l);
    if (listeners.size === 1) window.addEventListener("storage", fromOtherWindow);
    return () => {
      listeners.delete(l);
      if (listeners.size === 0) window.removeEventListener("storage", fromOtherWindow);
    };
  }, () => tabs);
}

/** A record page names its tab: the candidate's name, the job title. */
export function useTabName(href: string, label: string | undefined) {
  useEffect(() => { if (label) nameTab(href, label); }, [href, label]);
}

/** For tests: start with no tabs, as this user. */
export function resetTabs(who = "tnt_test:usr_test") { forgetTabs(); setTabsOwner(who); }
