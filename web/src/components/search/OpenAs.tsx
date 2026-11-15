// What clicking a candidate in the search results does: open their profile
// (the usual list), or read their full resume beside the results.
import { type ReactNode, useState } from "react";

export type OpenAs = "profile" | "resume";

// Kept on this device (a convenience only).
const KEY = "hiringplug.candidates.openAs";

function read(): OpenAs {
  try { return window.localStorage.getItem(KEY) === "resume" ? "resume" : "profile"; } catch { return "profile"; }
}

export function useOpenAs(): [OpenAs, (v: OpenAs) => void] {
  const [mode, setMode] = useState<OpenAs>(read);
  const choose = (v: OpenAs) => {
    setMode(v);
    try { window.localStorage.setItem(KEY, v); } catch { /* only a convenience */ }
  };
  return [mode, choose];
}

/** "Click opens [Profile] [Full resume]": a two-way switch. */
export function OpenAsSwitch({ value, onChange }: { value: OpenAs; onChange: (v: OpenAs) => void }) {
  const option = (v: OpenAs, label: string, icon: ReactNode) => (
    <button type="button" className={value === v ? "on" : undefined} aria-pressed={value === v} onClick={() => { onChange(v); }}>
      {icon}{label}
    </button>
  );
  return (
    <span className="open-as" role="group" aria-label="Clicking a candidate opens">
      <span aria-hidden="true">Click opens</span>
      <span className="seg">
        {option("profile", "Profile", <PersonIcon />)}
        {option("resume", "Full resume", <PageIcon />)}
      </span>
    </span>
  );
}

const PersonIcon = () => (
  <svg viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6"><circle cx="8" cy="5.5" r="2.8" /><path d="M2.5 14c.6-3 2.8-4.5 5.5-4.5s4.9 1.5 5.5 4.5" /></svg>
);
const PageIcon = () => (
  <svg viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6"><path d="M4 1.8h5.5L12.5 5v9.2H4z" /><path d="M6.2 8h4M6.2 10.5h4M6.2 5.5h2" /></svg>
);
