import { useState, type ReactNode } from "react";

import type { Permission } from "../api/client";
import type { Route } from "../routes";
import { groupSettings, searchSettings, settingsSearchEntries } from "./settingsNavigation";
import { focusCurrentSetting } from "./useSettingsAnchor";
import "./settings-shell.css";

export function SettingsLayout({ section, pages, permissions, children }: {
  section: string; pages: Route[]; permissions: readonly Permission[]; children: ReactNode;
}) {
  const [query, setQuery] = useState("");
  const groups = groupSettings(pages);
  const matches = searchSettings(settingsSearchEntries(groups, permissions), query);
  const searching = Boolean(query.trim());

  return (
    <div className="settings-layout">
      <div className="settings-header">
        <h1>Settings</h1>
        <form role="search" aria-label="Settings and features" onSubmit={(e) => {
          e.preventDefault();
          const first = matches[0];
          if (first) {
            setQuery("");
            if (window.location.hash === first.href) focusCurrentSetting();
            else window.location.hash = first.href;
          }
        }}>
          <label className="settings-search">
            <svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4 4" />
            </svg>
            <input type="search" aria-label="Search settings and features" placeholder="Search settings and features"
              value={query} onChange={(e) => { setQuery(e.target.value); }} onKeyDown={(e) => {
                if (e.key === "Escape" && query) {
                  e.stopPropagation();
                  setQuery("");
                }
              }} />
          </label>
        </form>
      </div>
      <div className="settings-search-results" hidden={!searching}>
        <p role="status">
          {matches.length ? `${String(matches.length)} ${matches.length === 1 ? "result" : "results"}` : "No matching settings or features"}
        </p>
        {matches.length > 0 && <ul className="search-hits" aria-label="Settings and feature results">
          {matches.map((item) => (
            <li key={item.href}>
              <a href={item.href} onClick={(e) => {
                if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
                setQuery("");
                if (window.location.hash === item.href) focusCurrentSetting();
              }}>
                <span className="search-name">{item.label}</span>
                <span className="search-detail">{item.group}</span>
              </a>
            </li>
          ))}
        </ul>}
      </div>
      <div className="settings-columns">
        <nav className="settings-nav" aria-label="Settings">
          {groups.map((group) => (
            <details key={group.label} open={group.pages.some((route) => section === route.section)}>
              <summary>{group.label}</summary>
              <div className="settings-links">
                {group.pages.map((route) => (
                  <a key={route.section} href={route.menu?.href} aria-current={section === route.section ? "page" : undefined}>{route.menu?.label}</a>
                ))}
              </div>
            </details>
          ))}
        </nav>
        <div className="settings-body">{children}</div>
      </div>
    </div>
  );
}
