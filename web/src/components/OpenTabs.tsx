// The strip of open records above the page: a tab per candidate, contact,
// company or job you opened, named after it, grouped by kind; the one you
// are on marked. Shift-click selects a run of tabs, Ctrl-click (Cmd on a
// Mac) picks several; then close them, or close all. With many open, each
// kind shows its latest few and "+N" lists the rest, and "All open" finds
// any by name. A tab lights green
// when something finished on its record (a CV formatted), until you open it.
// Ctrl-, Shift- or middle-click on a link to a record anywhere opens it as a
// tab here, behind the page you are on, never as a browser tab; middle-click
// on a tab here goes to it (dragging a link or a tab out of the window still
// makes a browser tab). The strip stays at the
// top as the page scrolls.
import { type MouseEvent, useEffect, useRef, useState } from "react";

import { useNotifications } from "./Notifications";
import { closeTabs, markTabDone, nameTab, openTab, recordOf, seenTab, type TabKind, useOpenTabs } from "./openRecords";
import { useCan } from "./useCan";

const KIND: Record<TabKind, { word: string; plural: string; list: string }> = {
  candidate: { word: "Candidate", plural: "Candidates", list: "#/candidates" },
  contact: { word: "Contact", plural: "Contacts", list: "#/contacts" },
  company: { word: "Company", plural: "Companies", list: "#/companies" },
  job: { word: "Job", plural: "Jobs", list: "#/jobs" },
};
const ORDER = Object.keys(KIND) as TabKind[];
/** Notifications that mean something finished on a record (its tab lights). */
const FINISHED = new Set(["cv_formatted"]);

/** Goes to another page of the app. */
function goTo(href: string) { window.location.hash = href; }

export function OpenTabs({ hash }: { hash: string }) {
  const tabs = useOpenTabs();
  const here = recordOf(hash);
  const [selected, setSelected] = useState<string[]>([]);
  const [anchor, setAnchor] = useState<string | null>(null);
  const [find, setFind] = useState("");
  const bell = useNotifications(useCan("notifications:read"));
  useEffect(() => { if (here) { openTab(here.href, here.kind); seenTab(here.href); } }, [here?.href, here?.kind]); // eslint-disable-line react-hooks/exhaustive-deps
  // Typed into a form on this page (not a search box): closing its tab asks first.
  const typed = useRef(false);
  useEffect(() => {
    typed.current = false;
    const onInput = (e: Event) => {
      const field = e.target as HTMLElement | null;
      if (field?.closest("form") && !field.closest(".open-tabs") && (field as HTMLInputElement).type !== "search") typed.current = true;
    };
    const onSave = () => { typed.current = false; }; // saved: nothing left unsaved
    document.addEventListener("input", onInput, true);
    document.addEventListener("submit", onSave, true);
    return () => { document.removeEventListener("input", onInput, true); document.removeEventListener("submit", onSave, true); };
  }, [hash]);
  // Something finished on an open record you are not looking at: its tab lights green.
  useEffect(() => {
    for (const it of bell.data?.items ?? []) {
      if (it.new && FINISHED.has(it.kind)) markTabDone(it.href, it.id, it.href === here?.href);
    }
  }, [bell.data, here?.href, tabs.length]);
  // Ctrl-, Shift- or middle-click on a record's link: a tab here, not a browser tab.
  useEffect(() => {
    const inBackground = (e: globalThis.MouseEvent) => {
      if (e.defaultPrevented || !(e.ctrlKey || e.metaKey || e.shiftKey || e.button === 1)) return;
      const link = (e.target as Element | null)?.closest("a[href]");
      if (!link) return;
      if (link.closest(".open-tabs")) {
        // Middle-click on a tab here goes to it; Ctrl and Shift select tabs.
        if (e.button === 1) { e.preventDefault(); window.location.assign(link.getAttribute("href") ?? "#/"); }
        return;
      }
      const record = recordOf(link.getAttribute("href") ?? "");
      if (!record) return;
      e.preventDefault();
      openTab(record.href, record.kind);
      nameTab(record.href, link.textContent);
    };
    document.addEventListener("click", inBackground, true);
    document.addEventListener("auxclick", inBackground, true);
    return () => { document.removeEventListener("click", inBackground, true); document.removeEventListener("auxclick", inBackground, true); };
  }, []);
  // Kept just under the top bar as the page scrolls (the bar wraps on a phone: its height is measured).
  const [under, setUnder] = useState(0);
  useEffect(() => {
    const bar = document.querySelector<HTMLElement>(".topbar");
    if (!bar || typeof ResizeObserver === "undefined") return undefined;
    const measure = () => { setUnder(bar.offsetHeight); };
    measure();
    const watch = new ResizeObserver(measure);
    watch.observe(bar);
    return () => { watch.disconnect(); };
  }, []);
  // A "+N" or "All open" list closes on a click elsewhere.
  useEffect(() => {
    const outside = (e: globalThis.MouseEvent) => {
      document.querySelectorAll<HTMLDetailsElement>(".open-tab-more[open]").forEach((d) => { if (!d.contains(e.target as Node)) d.open = false; });
    };
    document.addEventListener("click", outside);
    return () => { document.removeEventListener("click", outside); };
  }, []);
  if (tabs.length === 0) return null;
  // The order on screen: grouped by kind.
  const shown = ORDER.flatMap((k) => tabs.filter((t) => t.kind === k));
  const picked = selected.filter((h) => tabs.some((t) => t.href === h));

  const close = (hrefs: string[]) => {
    const leaving = !!here && hrefs.includes(here.href);
    if (leaving && typed.current && !window.confirm("Close this page? What you typed on it is not saved.")) return;
    closeTabs(hrefs);
    setSelected((s) => s.filter((h) => !hrefs.includes(h)));
    if (!here || !leaving) return;
    // The tab you are on: the one beside it opens, or its list.
    const at = shown.findIndex((t) => t.href === here.href);
    const rest = shown.filter((t) => !hrefs.includes(t.href));
    const next = rest[Math.min(at, rest.length - 1)];
    goTo(next ? next.href : KIND[here.kind].list);
  };
  const click = (e: MouseEvent, href: string) => {
    if (e.shiftKey) { // a run of tabs, from the last one clicked
      e.preventDefault();
      const from = shown.findIndex((t) => t.href === (anchor ?? here?.href));
      const to = shown.findIndex((t) => t.href === href);
      const [a, b] = from < 0 ? [to, to] : [Math.min(from, to), Math.max(from, to)];
      setSelected(shown.slice(a, b + 1).map((t) => t.href));
      return;
    }
    setAnchor(href);
    if (e.ctrlKey || e.metaKey) { // one more, or one less
      e.preventDefault();
      setSelected((s) => (s.includes(href) ? s.filter((h) => h !== href) : [...s, href]));
      return;
    }
    setSelected([]);
  };

  const tabEl = (t: (typeof tabs)[number], inMenu = false) => {
    const current = here?.href === t.href;
    const name = t.label || `${KIND[t.kind].word}…`;
    const isPicked = picked.includes(t.href);
    return (
      <span key={t.href} className={`open-tab${current ? " current" : ""}${isPicked ? " picked" : ""}${t.done && !current ? " done" : ""}`}>
        <a href={t.href} aria-current={current ? "page" : undefined}
          onClick={(e) => { click(e, t.href); if (inMenu && !e.defaultPrevented) e.currentTarget.closest("details")?.removeAttribute("open"); }}
          title={t.done && !current ? `${name}: finished (a CV is formatted)` : `${KIND[t.kind].word}: ${name}`}>
          {t.done && !current && <span className="open-tab-dot" aria-label="finished" />}
          <span className="open-tab-name">{name}</span>
          {isPicked && <span className="visually-hidden"> (selected)</span>}
        </a>
        <button type="button" className="open-tab-close" aria-label={`Close ${name}`} title="Close" onClick={() => { close([t.href]); }}>×</button>
      </span>
    );
  };
  const needle = find.trim().toLowerCase();
  const found = needle ? shown.filter((t) => t.label.toLowerCase().includes(needle)) : shown;

  return (
    <nav className="open-tabs" aria-label="Open records" style={{ top: under }}
      title="Shift-click selects a run of tabs; Ctrl-click (Cmd on a Mac) picks several"
      onKeyDown={(e) => { if (e.key === "Escape") setSelected([]); }}>
      <div className="open-tab-groups">
        {ORDER.filter((k) => tabs.some((t) => t.kind === k)).map((k) => {
          const { inline, more } = inlineTabs(tabs.filter((t) => t.kind === k), here?.href);
          return (
            <span key={k} className={`open-tab-group kind-${k}`} role="group" aria-label={KIND[k].plural}>
              <span className="open-tab-group-name" aria-hidden="true">{KIND[k].plural}</span>
              {inline.map((t) => tabEl(t))}
              {more.length > 0 && (
                <details className="open-tab-more">
                  <summary className={more.some((t) => t.done) ? "done" : undefined} aria-label={`${String(more.length)} more ${KIND[k].plural.toLowerCase()}`}
                    title={`${String(more.length)} more ${KIND[k].plural.toLowerCase()}`}>+{more.length}</summary>
                  <div className="open-tab-menu" role="dialog" aria-label={`More ${KIND[k].plural.toLowerCase()}`}>{more.map((t) => tabEl(t, true))}</div>
                </details>
              )}
            </span>
          );
        })}
      </div>
      <span className="open-tab-actions">
        {tabs.length > INLINE && (
          <details className="open-tab-more open-tab-all" onToggle={(e) => { if (!e.currentTarget.open) setFind(""); }}>
            <summary title="Find an open record">All open ({tabs.length})</summary>
            <div className="open-tab-menu" role="dialog" aria-label="All open records">
              <input type="search" aria-label="Find an open record" placeholder="Type a name…" value={find} autoFocus
                onChange={(e) => { setFind(e.target.value); }}
                onKeyDown={(e) => {
                  const first = found[0];
                  if (e.key === "Enter" && first) { e.currentTarget.closest("details")?.removeAttribute("open"); goTo(first.href); }
                }} />
              {found.length === 0 ? <p className="muted small">None open by that name.</p> : ORDER.map((k) => {
                const list = found.filter((t) => t.kind === k);
                return list.length > 0 && (
                  <div key={k} className={`open-tab-menu-group kind-${k}`}>
                    <span className="open-tab-group-name">{KIND[k].plural}</span>
                    {list.map((t) => tabEl(t, true))}
                  </div>
                );
              })}
            </div>
          </details>
        )}
        {picked.length > 0 ? (
          <>
            <span className="muted small">{picked.length} selected</span>
            <button type="button" className="link" onClick={() => { close(picked); }}>Close selected</button>
            <button type="button" className="link" onClick={() => { setSelected([]); }}>Clear</button>
          </>
        ) : (
          <button type="button" className="link" onClick={() => { setSelected(shown.map((t) => t.href)); }}>Select all</button>
        )}
        <button type="button" className="link" onClick={() => { close(shown.map((t) => t.href)); }}>Close all</button>
      </span>
    </nav>
  );
}

/** How many tabs of a kind show in the row; the others are under "+N". */
export const INLINE = 4;

/** A kind's tabs in the row (the latest opened, and the one you are on) and
 * the rest, for its "+N" list (newest first). Pure: unit tested. */
export function inlineTabs<T extends { href: string }>(list: T[], current?: string): { inline: T[]; more: T[] } {
  if (list.length <= INLINE + 1) return { inline: list, more: [] };
  const keep = new Set(list.slice(-INLINE).map((t) => t.href));
  if (current && list.some((t) => t.href === current) && !keep.has(current)) keep.add(current);
  return { inline: list.filter((t) => keep.has(t.href)), more: list.filter((t) => !keep.has(t.href)).reverse() };
}
