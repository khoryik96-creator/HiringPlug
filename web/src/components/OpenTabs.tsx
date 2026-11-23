// The strip of open records above the page: a tab per candidate, contact,
// company or job you opened, named after it; the one you are on marked.
import { useEffect } from "react";

import { closeTab, openTab, recordOf, type TabKind, useOpenTabs } from "./openTabs";

const KIND: Record<TabKind, { word: string; plural: string; list: string }> = {
  candidate: { word: "Candidate", plural: "Candidates", list: "#/candidates" },
  contact: { word: "Contact", plural: "Contacts", list: "#/contacts" },
  company: { word: "Company", plural: "Companies", list: "#/companies" },
  job: { word: "Job", plural: "Jobs", list: "#/jobs" },
};

/** The strip's look, while the designs are compared (A tabs, B pills, C grouped by kind). */
export type TabsStyle = "tabs" | "pills" | "grouped";

const initials = (name: string) => name.split(/[\s·]+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join("");

export function OpenTabs({ hash, look = "tabs" }: { hash: string; look?: TabsStyle }) {
  const tabs = useOpenTabs();
  const here = recordOf(hash);
  useEffect(() => { if (here) openTab(here.href, here.kind); }, [here?.href, here?.kind]); // eslint-disable-line react-hooks/exhaustive-deps
  if (tabs.length === 0) return null;
  const close = (href: string) => {
    const at = tabs.findIndex((t) => t.href === href);
    closeTab(href);
    if (here?.href !== href) return;
    // The tab you are on: the one beside it opens, or the list.
    const next = tabs[at + 1] ?? tabs[at - 1];
    window.location.assign(next ? next.href : KIND[tabs[at]?.kind ?? "candidate"].list);
  };
  const tab = (t: (typeof tabs)[number]) => {
    const current = here?.href === t.href;
    const name = t.label || `${KIND[t.kind].word}…`;
    return (
      <span key={t.href} className={`open-tab kind-${t.kind}${current ? " current" : ""}`}>
        <a href={t.href} aria-current={current ? "page" : undefined} title={`${KIND[t.kind].word}: ${name}`}>
          <span className="open-tab-kind" aria-hidden="true">{look === "pills" && t.label ? initials(t.label) : KIND[t.kind].word[0]}</span>
          <span className="open-tab-name">{name}</span>
        </a>
        <button type="button" className="open-tab-close" aria-label={`Close ${name}`} title="Close" onClick={() => { close(t.href); }}>×</button>
      </span>
    );
  };
  const kinds = (Object.keys(KIND) as TabKind[]).filter((k) => tabs.some((t) => t.kind === k));
  return (
    <nav className={`open-tabs look-${look}`} aria-label="Open records">
      {look === "grouped"
        ? kinds.map((k) => (
          <span key={k} className={`open-tab-group kind-${k}`}>
            <span className="open-tab-group-name">{KIND[k].plural}</span>
            {tabs.filter((t) => t.kind === k).map(tab)}
          </span>
        ))
        : tabs.map(tab)}
    </nav>
  );
}
