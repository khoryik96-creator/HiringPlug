import { useState } from "react";

import type { Application, Schemas } from "../api/client";
import { CHANNEL, CLOSE_REASON, keysOf, OPEN_STAGES, STAGE } from "../labels";
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

/**
 * A candidate's card on a job's board: name, time in the stage and
 * fit. Clicking the card chooses it (for emailing or moving the chosen ones
 * from the bar above the board), as does Space on its name; drag it to
 * another column to move it.
 */
export function ApplicationCard({ app, match, onShowFit, picked, onPick, canMove, onDragStart }: {
  app: Application;
  /** Chosen, for the bar above the board. */
  picked?: boolean;
  onPick?: (app: Application, on: boolean) => void;
  match?: Match;
  onShowFit?: (app: Application) => void;
  canMove: boolean;
  onDragStart: (app: Application) => void;
}) {
  return (
    <div
      className={`card${picked ? " picked" : ""}${onPick ? " pickable" : ""}`}
      draggable={canMove}
      onDragStart={(e) => { e.dataTransfer.setData("text/plain", app.id); onDragStart(app); }}
      onClick={(e) => {
        // The name opens the candidate and the fit opens its reasons; anywhere else ticks the card.
        if (!onPick || (e.target as HTMLElement).closest("a, button, input, select, label")) return;
        onPick(app, !picked);
      }}
    >
      <span className="card-name">
        <a href={`#/candidates/${app.candidateId}`}
          onKeyDown={(e) => { if (onPick && e.key === " ") { e.preventDefault(); onPick(app, !picked); } }}>
          {app.candidateName}
        </a>
        {picked && <span className="visually-hidden"> (chosen)</span>}
      </span>
      <div className="muted">{timeInStage(app)} in {STAGE[app.stage].toLowerCase()}{app.channel ? ` · via ${CHANNEL[app.channel]}` : ""}</div>
      {match && (
        <button type="button" className="link" aria-label={`Fit of ${app.candidateName}: why?`} onClick={() => onShowFit?.(app)}>
          <FitBadge match={match} />
        </button>
      )}
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
