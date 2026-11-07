// Settings: one place for everything the company and each person set up.
// The top bar's Settings button leads here; a menu on the left lists the
// parts the signed-in user may change (from the route table, src/routes.tsx),
// and each part keeps its own address.
import type { ReactNode } from "react";

import type { Route } from "../routes";

export function SettingsLayout({ section, pages, children }: { section: string; pages: Route[]; children: ReactNode }) {
  return (
    <div className="settings-layout">
      <nav className="settings-nav" aria-label="Settings">
        <h2>Settings</h2>
        {pages.map((r) => (
          <a key={r.section} href={r.menu?.href} aria-current={section === r.section ? "page" : undefined}>{r.menu?.label}</a>
        ))}
      </nav>
      <div className="settings-body">{children}</div>
    </div>
  );
}
