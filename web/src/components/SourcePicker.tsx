// A candidate's source as a row of icons side by side, instead of a dropdown.
import type { ReactElement } from "react";

import { useFieldSettings } from "./useFieldSettings";

const svg = (path: ReactElement) => (
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{path}</svg>
);

/** Icons for the sources that come with the app; a company's own get their initials. */
const ICONS: Record<string, ReactElement> = {
  jobstreet: <span className="source-badge" style={{ background: "#1c3f94" }}>JS</span>,
  linkedin: <span className="source-badge" style={{ background: "#0a66c2" }}>in</span>,
  referral: svg(<><circle cx="9" cy="8" r="3" /><circle cx="17" cy="9" r="2.5" /><path d="M3 20c0-3.3 2.7-6 6-6s6 2.7 6 6M15 14.5c3 0 6 2 6 5.5" /></>),
  careers_page: svg(<><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3c2.5 2.7 2.5 15.3 0 18M12 3c-2.5 2.7-2.5 15.3 0 18" /></>),
  direct: svg(<><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3 7l9 6 9-6" /></>),
  other: svg(<><circle cx="6" cy="12" r="1.3" /><circle cx="12" cy="12" r="1.3" /><circle cx="18" cy="12" r="1.3" /></>),
};

function initials(label: string): string {
  return label.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? "").join("") || "?";
}

export function SourceIcon({ source, label }: { source: string; label: string }) {
  return ICONS[source] ?? <span className="source-badge" style={{ background: "#6b7280" }}>{initials(label)}</span>;
}

/** Pick one source; picking the chosen one again clears it. */
export function SourcePicker({ value, onChange, label = "Source", invalid = false }: { value: string; onChange: (v: string) => void; label?: string; invalid?: boolean }) {
  const fs = useFieldSettings();
  return (
    <div className="source-picker" role="group" aria-label={label} aria-invalid={invalid || undefined}>
      {fs.options("candidate_source", value || null).map((o) => (
        <button key={o.key} type="button" aria-pressed={value === o.key} title={o.label}
          onClick={() => { onChange(value === o.key ? "" : o.key); }}>
          <SourceIcon source={o.key} label={o.label} />
          <span>{o.label}</span>
        </button>
      ))}
    </div>
  );
}
