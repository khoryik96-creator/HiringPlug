import { useState } from "react";

import type { Application, Schemas } from "../api/client";
import { CHANNEL, CLOSE_REASON, keysOf, OPEN_STAGES, STAGE } from "../labels";
import { Initials } from "./Initials";
import { FitBadge, type Match } from "./Fit";
import type { MoveRequest } from "./useMoveApplication";

type Stage = Schemas["Stage"];
const isClosed = (s: Stage) => s === "rejected" || s === "withdrawn";

/** The "Move to…" menu of an application, with the reason asked when closing it. */
export function MoveMenu({ app, onMove, busy }: { app: Application; onMove: (m: MoveRequest) => void; busy: boolean }) {
  const [closing, setClosing] = useState<Stage | null>(null);
  const [reason, setReason] = useState<"" | Schemas["CloseReason"]>("");
  const [note, setNote] = useState("");
  const options = [...OPEN_STAGES, "rejected", "withdrawn"].filter((s) => s !== app.stage) as Stage[];

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
        const to = e.target.value as Stage;
        if (isClosed(to)) setClosing(to);
        else onMove({ app, stage: to });
      }}
    >
      <option value="">{isClosed(app.stage) ? "Reopen at…" : "Move to…"}</option>
      {options.map((s) => <option key={s} value={s}>{STAGE[s]}</option>)}
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

const when = new Intl.DateTimeFormat(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
const day = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });

/** Waiting in a stage: grey, amber after 7 days, red after 14 (not once placed). */
function waitTone(app: Application, days: number): string {
  if (app.stage === "placed") return "";
  return days >= 14 ? " red" : days >= 7 ? " amber" : "";
}

function offerChip(expiresOn: string, now: number) {
  // Calendar days from today to the last day (local dates, so "tomorrow" means tomorrow).
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  const left = Math.round((Date.parse(`${expiresOn}T00:00:00`) - today.getTime()) / 86_400_000);
  const text = left <= 0 ? "answer due today" : left === 1 ? "answer by tomorrow" : `answer by ${day.format(new Date(`${expiresOn}T12:00:00`))}`;
  return <span className={`chip${left <= 1 ? " red" : left <= 3 ? " amber" : ""}`}>{text}</span>;
}

/**
 * A candidate's card on a job's board: initials, name, current title, how
 * long they have waited in the stage (amber after 7 days, red after 14),
 * their next interview, an offer's deadline and fit. Clicking the card
 * chooses it (for emailing or moving the chosen ones from the bar above the
 * board), as does Space on its name; drag it to another column to move it.
 */
export function ApplicationCard({ app, match, onShowFit, picked, onPick, canMove, onDragStart, upcoming, now = 0 }: {
  app: Application;
  /** Chosen, for the bar above the board. */
  picked?: boolean;
  onPick?: (app: Application, on: boolean) => void;
  match?: Match;
  onShowFit?: (app: Application) => void;
  canMove: boolean;
  onDragStart: (app: Application) => void;
  upcoming?: Upcoming;
  /** The time the board was shown (ms). */
  now?: number;
}) {
  const days = daysInStage(app, now);
  return (
    <div
      className={`card app-card${picked ? " picked" : ""}${onPick ? " pickable" : ""}`}
      draggable={canMove}
      onDragStart={(e) => { e.dataTransfer.setData("text/plain", app.id); onDragStart(app); }}
      onClick={(e) => {
        // The name opens the candidate and the fit opens its reasons; anywhere else ticks the card.
        if (!onPick || (e.target as HTMLElement).closest("a, button, input, select, label")) return;
        onPick(app, !picked);
      }}
    >
      <div className="who">
        <Initials name={app.candidateName} />
        <div className="who-text">
          <span className="card-name">
            <a href={`#/candidates/${app.candidateId}`} title={app.candidateName}
              onKeyDown={(e) => { if (onPick && e.key === " ") { e.preventDefault(); onPick(app, !picked); } }}>
              {app.candidateName}
            </a>
            {picked && <span className="visually-hidden"> (chosen)</span>}
          </span>
          {app.candidateTitle && <div className="who-title">{app.candidateTitle}</div>}
        </div>
      </div>
      <div className="chips">
        <span className={`chip${waitTone(app, days)}`} title={`In ${STAGE[app.stage].toLowerCase()} since ${day.format(new Date(app.stageChangedAt))}`}>
          {timeInStage(app, now)}
        </span>
        {upcoming?.interviewAt && <span className="chip cal">{when.format(new Date(upcoming.interviewAt))}</span>}
        {upcoming?.offerExpiresOn && offerChip(upcoming.offerExpiresOn, now)}
        {app.channel && <span className="chip">via {CHANNEL[app.channel]}</span>}
        {match && (
          <button type="button" className="link" aria-label={`Fit of ${app.candidateName}: why?`} onClick={() => onShowFit?.(app)}>
            <FitBadge match={match} />
          </button>
        )}
      </div>
    </div>
  );
}

/** "Move to…" for the chosen cards, asking the reason once when closing them. */
export function MoveChosen({ apps, onMove, busy }: { apps: Application[]; onMove: (m: Omit<MoveRequest, "app">) => void; busy: boolean }) {
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
      onChange={(e) => { const to = e.target.value as Stage; if (isClosed(to)) setClosing(to); else onMove({ stage: to }); }}>
      <option value="">Move to…</option>
      {([...OPEN_STAGES, "rejected", "withdrawn"] as Stage[]).map((s) => <option key={s} value={s}>{STAGE[s]}</option>)}
    </select>
  );
}
