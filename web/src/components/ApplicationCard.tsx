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
        <select aria-label="Reason" value={reason} onChange={(e) => { setReason(e.target.value as typeof reason); }}>
          <option value="">Why {closing}?</option>
          {keysOf(CLOSE_REASON).map((r) => <option key={r} value={r}>{CLOSE_REASON[r]}</option>)}
        </select>
        <input aria-label="Note (optional)" placeholder="Note (optional)" value={note} onChange={(e) => { setNote(e.target.value); }} />
        <div className="row">
          <button type="submit" disabled={!reason || busy}>{STAGE[closing]}</button>
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

/** A candidate's card on a job's board: drag it to another column, or use the menu. Its fit badge opens the reasons. */
export function ApplicationCard({ app, match, onShowFit, onEmailClient, picked, onPick, canMove, onMove, busy, onDragStart }: {
  app: Application;
  /** Chosen for an email to the client about several candidates. */
  picked?: boolean;
  onPick?: (app: Application, on: boolean) => void;
  match?: Match;
  onShowFit?: (app: Application) => void;
  /** Opens an email to the client about this candidate. */
  onEmailClient?: (app: Application) => void;
  canMove: boolean;
  onMove: (m: MoveRequest) => void;
  busy: boolean;
  onDragStart: (app: Application) => void;
}) {
  return (
    <div
      className={picked ? "card picked" : "card"}
      draggable={canMove}
      onDragStart={(e) => { e.dataTransfer.setData("text/plain", app.id); onDragStart(app); }}
    >
      <span className="card-name">
        {onPick && (
          <input type="checkbox" aria-label={`Choose ${app.candidateName} for an email to the client`} checked={picked ?? false}
            onChange={(e) => { onPick(app, e.target.checked); }} />
        )}
        <a href={`#/candidates/${app.candidateId}`}>{app.candidateName}</a>
      </span>
      {app.candidateTitle && <div className="muted">{app.candidateTitle}</div>}
      <div className="muted">{timeInStage(app)} in {STAGE[app.stage].toLowerCase()}</div>
      {app.channel && <div className="muted">Applied via {CHANNEL[app.channel]}</div>}
      {match && (
        <button type="button" className="link" aria-label={`Fit of ${app.candidateName}: why?`} onClick={() => onShowFit?.(app)}>
          <FitBadge match={match} />
        </button>
      )}
      {canMove && <MoveMenu app={app} onMove={onMove} busy={busy} />}
      {onEmailClient && (
        <button type="button" className="link" aria-label={`Email client about ${app.candidateName}`} onClick={() => { onEmailClient(app); }}>Email client</button>
      )}
    </div>
  );
}
