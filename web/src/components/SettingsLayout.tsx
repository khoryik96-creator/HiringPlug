// Settings: one place for everything the company and each person set up.
// The top bar's Settings button leads here; a menu on the left lists the
// parts the signed-in user may change, and each part keeps its own address.
import type { ReactNode } from "react";

import type { Permission } from "../api/client";

/** Each part of Settings: address, label, the permission needed (none: everyone). */
export const SETTINGS_PAGES: [string, string, Permission | null][] = [
  ["settings", "General", "settings:manage"],
  ["ai-provider", "AI provider", "aiproviders:manage"],
  ["team", "Team", "identity.team:manage"],
  ["access", "Roles and access", "identity.roles:manage"],
  ["privacy", "Data protection", "privacy:manage"],
  ["integrations", "Integrations", "integrations:manage"],
  ["cv-templates", "CV templates", "cvformats:create"],
  ["email-templates", "Email templates", "emails:send"],
  ["account", "Your account", null],
];

/** The parts of Settings this user may open, in menu order. */
export function settingsPages(permissions: readonly Permission[]) {
  return SETTINGS_PAGES.filter(([, , permission]) => permission === null || permissions.includes(permission));
}

export function SettingsLayout({ section, permissions, children }: { section: string; permissions: readonly Permission[]; children: ReactNode }) {
  return (
    <div className="settings-layout">
      <nav className="settings-nav" aria-label="Settings">
        <h2>Settings</h2>
        {settingsPages(permissions).map(([key, label]) => (
          <a key={key} href={`#/${key}`} aria-current={section === key ? "page" : undefined}>{label}</a>
        ))}
      </nav>
      <div className="settings-body">{children}</div>
    </div>
  );
}
