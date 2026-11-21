// The parts of an Outlook-style event composer, shared by appointments and
// interviews: a toolbar, a large title, rows led by an icon (people, time,
// online meeting, place), and the body.
import type { ReactNode } from "react";
import { useState } from "react";

import { CloseIcon } from "../ActionIcons";
import { linkProblem, NEW_MEETING, SERVICE_LABEL, SHORT_LABEL, type Service } from "./meeting";
import "./event-composer.css";

const Svg = ({ children }: { children: ReactNode }) => (
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{children}</svg>
);
export const PeopleIcon = () => <Svg><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20a6.5 6.5 0 0 1 13 0" /><path d="M16 4.6a3.5 3.5 0 0 1 0 6.8M18 14.2A6.5 6.5 0 0 1 21.5 20" /></Svg>;
export const TimeIcon = () => <Svg><circle cx="12" cy="12" r="8.5" /><path d="M12 7v5l3.2 2" /></Svg>;
export const VideoIcon = () => <Svg><rect x="2.5" y="6" width="13" height="12" rx="2.5" /><path d="m15.5 10.5 6-3.5v10l-6-3.5" /></Svg>;
export const PinIcon = () => <Svg><path d="M12 21s-6.5-6-6.5-11a6.5 6.5 0 0 1 13 0c0 5-6.5 11-6.5 11Z" /><circle cx="12" cy="10" r="2.3" /></Svg>;
export const BellIcon = () => <Svg><path d="M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15Z" /><path d="M10 20.5a2.2 2.2 0 0 0 4 0" /></Svg>;
export const SendIcon = () => <Svg><path d="M3.5 11.5 20.5 4l-6.8 16.5-2.6-6.6Z" /><path d="m11.1 13.9 9.4-9.9" /></Svg>;
export const NotesIcon = () => <Svg><path d="M5 5h14M5 10h14M5 15h9" /></Svg>;

/** The services' own colours, on a small camera mark. */
const MARK: Record<Service, string> = { teams: "#5059c9", google_meet: "#0f9d58", zoom: "#2d8cff" };
export function ServiceMark({ service }: { service: Service }) {
  return (
    <span className="service-mark" style={{ background: MARK[service] }} aria-hidden="true">
      <svg viewBox="0 0 24 24" width="12" height="12" fill="#fff"><rect x="3" y="7" width="11" height="10" rx="2" /><path d="m14 11 6-3.5v9L14 13Z" /></svg>
    </span>
  );
}

/** The composer: a toolbar (actions first, then options), the form, and a side panel. */
export function Composer({ label, toolbar, onClose, closing, side, children, onSubmit }: {
  label: string; toolbar: ReactNode; onClose: () => void; closing?: boolean; side?: ReactNode; children: ReactNode; onSubmit: () => void;
}) {
  return (
    <form className="composer" aria-label={label} onSubmit={(e) => { e.preventDefault(); onSubmit(); }}
      onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); onClose(); } }}>
      <div className="composer-bar" role="toolbar" aria-label="Actions">
        {toolbar}
        <button type="button" className="composer-close" aria-label="Close" title="Close" disabled={closing} onClick={onClose}><CloseIcon /></button>
      </div>
      <div className={`composer-body${side ? " with-side" : ""}`}>
        <div className="composer-main">{children}</div>
        {side && <aside className="composer-side">{side}</aside>}
      </div>
    </form>
  );
}

/** One row: an icon, then its fields. */
export function Row({ icon, label, children, className = "" }: { icon: ReactNode; label: string; children: ReactNode; className?: string }) {
  return (
    <div className={`composer-row ${className}`} role="group" aria-label={label}>
      <span className="composer-icon" title={label}>{icon}</span>
      <div className="composer-fields">{children}</div>
    </div>
  );
}

const pad = (n: number) => String(n).padStart(2, "0");
/** "HH:MM" plus minutes, as "HH:MM" and how many days later. */
export function addMinutes(time: string, minutes: number): { time: string; days: number } {
  const [h = 0, m = 0] = time.split(":").map(Number);
  const total = h * 60 + m + minutes;
  const day = 24 * 60;
  return { time: `${pad(Math.floor((((total % day) + day) % day) / 60))}:${pad((((total % day) + day) % day) % 60)}`, days: Math.floor(total / day) };
}
/** "HH:MM" as the user's clock shows it. */
export function clock(time: string): string {
  const [h = 0, m = 0] = time.split(":").map(Number);
  return new Date(2000, 0, 1, h, m).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}
/** "30 min", "1 hr", "1.5 hrs". */
export function lasting(minutes: number): string {
  if (minutes < 60) return `${String(minutes)} min`;
  const hours = minutes / 60;
  return `${String(Math.round(hours * 100) / 100)} ${hours === 1 ? "hr" : "hrs"}`;
}

/** A start time, every 15 minutes (and the time itself when it is between). */
export function StartTime({ value, onChange, label = "Start time" }: { value: string; onChange: (v: string) => void; label?: string }) {
  const times = Array.from({ length: 96 }, (_, i) => `${pad(Math.floor(i / 4))}:${pad((i % 4) * 15)}`);
  if (!times.includes(value)) times.push(value);
  return (
    <select className="composer-time" aria-label={label} value={value} onChange={(e) => { onChange(e.target.value); }}>
      {times.sort().map((t) => <option key={t} value={t}>{clock(t)}</option>)}
    </select>
  );
}

/** The end, as Outlook offers it: a time after the start, with how long that is. */
export function EndTime({ start, minutes, onChange, choices }: { start: string; minutes: number; onChange: (m: number) => void; choices: number[] }) {
  const all = choices.includes(minutes) ? choices : [...choices, minutes].sort((a, b) => a - b);
  return (
    <select className="composer-time" aria-label="End time" value={minutes} onChange={(e) => { onChange(Number(e.target.value)); }}>
      {all.map((m) => {
        const end = addMinutes(start, m);
        return <option key={m} value={m}>{clock(end.time)}{end.days > 0 ? " (next day)" : ""} ({lasting(m)})</option>;
      })}
    </select>
  );
}

/** Choosing the online meeting, then its link (or, for Teams through Outlook, nothing to paste). */
export function MeetingPicker({ online, link, onChange, teamsAuto, extra }: {
  online: string; link: string; onChange: (online: string, link: string) => void; teamsAuto: boolean;
  extra?: { value: string; label: string; icon?: ReactNode }[];
}) {
  const [touched, setTouched] = useState(false);
  const [pasteTeams, setPasteTeams] = useState(false);
  const service = (["teams", "google_meet", "zoom"] as string[]).includes(online) ? (online as Service) : null;
  const problem = service ? linkProblem(service, link) : "";
  const teamsByOutlook = service === "teams" && teamsAuto && !link && !pasteTeams;
  const pasting = service && !teamsByOutlook;
  return (
    <>
      <div className="meeting-choices" role="radiogroup" aria-label="Online meeting">
        {(["teams", "google_meet", "zoom"] as Service[]).map((s) => (
          <button key={s} type="button" role="radio" aria-checked={online === s} className="meeting-choice"
            onClick={() => { onChange(s, online === s ? link : ""); }}>
            <ServiceMark service={s} /> {SHORT_LABEL[s]}
          </button>
        ))}
        {(extra ?? [{ value: "none", label: "None" }]).map((x) => (
          <button key={x.value} type="button" role="radio" aria-checked={online === x.value} className="meeting-choice"
            onClick={() => { onChange(x.value, ""); }}>{x.icon}{x.label}</button>
        ))}
      </div>
      {teamsByOutlook && (
        <p className="composer-hint">Outlook adds the Teams link when it sends the invitation.{" "}
          <button type="button" className="link" onClick={() => { setPasteTeams(true); }}>Paste a link instead</button></p>
      )}
      {pasting && (
        <div className="meeting-link">
          <input type="url" aria-label={`${SERVICE_LABEL[service]} link`} placeholder={`Paste the ${SERVICE_LABEL[service]} link`}
            value={link} maxLength={480} aria-invalid={touched && problem ? true : undefined}
            autoFocus={pasteTeams && !link} onBlur={() => { setTouched(true); }}
            onChange={(e) => { onChange(service, e.target.value); }} />
          <a className="button" href={NEW_MEETING[service].url} target="_blank" rel="noreferrer">{NEW_MEETING[service].label} ↗</a>
        </div>
      )}
      {touched && problem && <small className="error" role="alert">{problem}</small>}
    </>
  );
}

const EMAIL = /^[^\s@<>(),;:"]+@[^\s@<>(),;:"]+\.[^\s@<>(),;:"]+$/;

/** People as chips; typing an address and Enter, comma or Tab adds it. */
export function EmailChips({ emails, onChange, disabled, invalid, label = "Attendees", placeholder = "Invite attendees", optional = false }: {
  emails: string[]; onChange: (e: string[]) => void; disabled?: boolean; invalid?: boolean; label?: string; placeholder?: string; optional?: boolean;
}) {
  const [draft, setDraft] = useState("");
  const [bad, setBad] = useState<string[]>([]);
  // Addresses become chips, once each; anything else stays typed, and is named.
  const add = () => {
    const typed = draft.split(/[,;\s]+/).map((e) => e.trim()).filter(Boolean);
    const known = new Set(emails.map((e) => e.toLowerCase()));
    const good: string[] = [];
    const wrong: string[] = [];
    for (const e of typed) {
      if (!EMAIL.test(e)) wrong.push(e);
      else if (!known.has(e.toLowerCase())) { known.add(e.toLowerCase()); good.push(e); }
    }
    if (good.length) onChange([...emails, ...good]);
    setBad(wrong);
    setDraft(wrong.join(" "));
  };
  return (
    <div className="chips-input">
      {emails.map((e) => (
        <span key={e} className={`person-chip${optional ? " optional" : ""}`}>{e}
          {!disabled && <button type="button" aria-label={`Remove ${e}`} onClick={() => { onChange(emails.filter((x) => x !== e)); }}>×</button>}
        </span>
      ))}
      <input aria-label={label} placeholder={emails.length ? "" : placeholder} value={draft} disabled={disabled} type="text"
        aria-invalid={invalid || bad.length > 0 ? true : undefined}
        onChange={(e) => { setDraft(e.target.value); }} onBlur={add}
        onKeyDown={(e) => {
          if ((e.key === "Enter" || e.key === "," || e.key === ";" || e.key === "Tab") && draft.trim()) { if (e.key !== "Tab") e.preventDefault(); add(); }
          if (e.key === "Backspace" && !draft && emails.length) onChange(emails.slice(0, -1));
        }} />
      {bad.length > 0 && <small className="error chips-error" role="alert">Not an email address: {bad.join(", ")}</small>}
    </div>
  );
}
