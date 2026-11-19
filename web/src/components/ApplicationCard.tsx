import { useState } from "react";

import type { Application, Schemas } from "../api/client";
import { CLOSE_REASON, keysOf, OPEN_STAGES, STAGE } from "../labels";
import { type Column, columnOf } from "./stages/columns";
import type { MoveRequest } from "./useMoveApplication";

type Stage = Schemas["Stage"];
const isClosed = (s: Stage) => s === "rejected" || s === "withdrawn";

/** Where an application can be moved: the job's own stages when known, else the standard ones. */
function targets(columns?: Column[]): Column[] {
  return columns ?? OPEN_STAGES.map((st) => ({ key: st, label: STAGE[st], stage: st }));
}

/** The "Move to…" menu of an application, with the reason asked when closing it. */
export function MoveMenu({ app, onMove, busy, columns }: {
  app: Application; onMove: (m: MoveRequest) => void; busy: boolean;
  /** The job's stages (the board's columns); the standard stages without. */
  columns?: Column[];
}) {
  const [closing, setClosing] = useState<Stage | null>(null);
  const [reason, setReason] = useState<"" | Schemas["CloseReason"]>("");
  const [note, setNote] = useState("");
  const here = columns ? columnOf(app, columns) : undefined;
  const open = targets(columns).filter((c) => (columns ? c.key !== here?.key : c.stage !== app.stage));
  const closes = (["rejected", "withdrawn"] as Stage[]).filter((s) => s !== app.stage);

  if (closing) {
    return (
      <form
        className="close-form"
        aria-label={`${STAGE[closing]}: why?`}
        onSubmit={(e) => {
          e.preventDefault();
          if (reason) onMove({ app, stage: closing, reason, reasonNote: note });
        }}
      >
        <select required aria-label="Reason" value={reason} onChange={(e) => { setReason(e.target.value as typeof reason); }}>
          <option value="">Why {closing}?</option>
          {keysOf(CLOSE_REASON).map((r) => <option key={r} value={r}>{CLOSE_REASON[r]}</option>)}
        </select>
        <input aria-label="Note (optional)" placeholder="Note (optional)" value={note} onChange={(e) => { setNote(e.target.value); }} />
        <div className="row">
          <button type="submit" disabled={busy}>{STAGE[closing]}</button>
          <button type="button" className="link" onClick={() => { setClosing(null); }}>Cancel</button>
        </div>
      </form>
    );
  }
  return (
    <select
      aria-label={`Move ${app.candidateName} on ${app.jobTitle}`}
      value=""
      disabled={busy}
      onChange={(e) => {
        const value = e.target.value;
        if (value === "rejected" || value === "withdrawn") { setClosing(value); return; }
        const to = open.find((c) => c.key === value);
        if (to) onMove({ app, stage: to.stage, step: to.step });
      }}
    >
      <option value="">{isClosed(app.stage) ? "Reopen at…" : "Move to…"}</option>
      {open.map((c) => <option key={c.key} value={c.key}>{c.label}</option>)}
      {closes.map((s) => <option key={s} value={s}>{STAGE[s]}</option>)}
    </select>
  );
}

/** How long an application has been in its stage, e.g. "3 days". */
export function timeInStage(app: Application, now = Date.now()): string {
  const days = Math.floor((now - Date.parse(app.stageChangedAt)) / 86_400_000);
  if (days < 1) return "today";
  return days === 1 ? "1 day" : `${String(days)} days`;
}

/** Days since it entered its stage. */
export function daysInStage(app: Application, now = Date.now()): number {
  return Math.max(0, Math.floor((now - Date.parse(app.stageChangedAt)) / 86_400_000));
}

/** What a card says is coming up for its candidate. */
export interface Upcoming {
  interviewAt?: string; // the next scheduled interview
  offerExpiresOn?: string; // a sent offer's last day to answer (YYYY-MM-DD)
}

export const when = new Intl.DateTimeFormat(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
const day = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });

/** Waiting in a stage: grey, amber after 7 days, red after 14 (not once placed). */
export function waitTone(app: Application, days: number): string {
  if (app.stage === "placed") return "";
  return days >= 14 ? " red" : days >= 7 ? " amber" : "";
}

export function offerChip(expiresOn: string, now: number) {
  // Calendar days from today to the last day (local dates, so "tomorrow" means tomorrow).
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  const left = Math.round((Date.parse(`${expiresOn}T00:00:00`) - today.getTime()) / 86_400_000);
  const text = left <= 0 ? "answer due today" : left === 1 ? "answer by tomorrow" : `answer by ${day.format(new Date(`${expiresOn}T12:00:00`))}`;
  return <span className={`chip${left <= 1 ? " red" : left <= 3 ? " amber" : ""}`}>{text}</span>;
}


/** "Move to…" for the chosen cards, asking the reason once when closing them. */
export function MoveChosen({ apps, onMove, busy, columns }: {
  apps: Application[]; onMove: (m: Omit<MoveRequest, "app">) => void; busy: boolean;
  /** The job's stages (the board's columns); the standard stages without. */
  columns?: Column[];
}) {
  const [closing, setClosing] = useState<Stage | null>(null);
  const [reason, setReason] = useState<"" | Schemas["CloseReason"]>("");
  const [note, setNote] = useState("");
  // Each batch is asked afresh: the last one's reason and note never carry over.
  const reset = () => { setClosing(null); setReason(""); setNote(""); };
  if (closing) {
    return (
      <form className="close-form row" aria-label={`${STAGE[closing]}: why?`}
        onSubmit={(e) => { e.preventDefault(); if (reason) { onMove({ stage: closing, reason, reasonNote: note }); reset(); } }}>
        <select required aria-label="Reason" value={reason} onChange={(e) => { setReason(e.target.value as typeof reason); }}>
          <option value="">Why {closing}?</option>
          {keysOf(CLOSE_REASON).map((r) => <option key={r} value={r}>{CLOSE_REASON[r]}</option>)}
        </select>
        <input aria-label="Note (optional)" placeholder="Note (optional)" value={note} onChange={(e) => { setNote(e.target.value); }} />
        <button type="submit" disabled={busy}>{STAGE[closing]} {apps.length}</button>
        <button type="button" className="link" onClick={reset}>Cancel</button>
      </form>
    );
  }
  return (
    <select aria-label="Move the chosen candidates" value="" disabled={busy}
      onChange={(e) => {
        const value = e.target.value;
        if (value === "rejected" || value === "withdrawn") { setClosing(value); return; }
        const to = targets(columns).find((c) => c.key === value);
        if (to) onMove({ stage: to.stage, step: to.step });
      }}>
      <option value="">Move to…</option>
      {targets(columns).map((c) => <option key={c.key} value={c.key}>{c.label}</option>)}
      {(["rejected", "withdrawn"] as Stage[]).map((s) => <option key={s} value={s}>{STAGE[s]}</option>)}
    </select>
  );
}
