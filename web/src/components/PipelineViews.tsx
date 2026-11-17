// A job's pipeline in two views (the switch above it, remembered):
// - Steps: the stages as a funnel to click, flagging who has waited long;
//   below, that stage's people, each with the button that moves them on.
// - Board: a column per stage, each person on one line (the longest waiting
//   first), the column folded after 8, a "next" button on each line.
// Clicking a person's line chooses them for the bar above (email, move), as
// the cards did.
import { useState } from "react";

import type { Application, Schemas } from "../api/client";
import { CHANNEL, OPEN_STAGES, STAGE } from "../labels";
import { daysInStage, MoveMenu, offerChip, timeInStage, type Upcoming, waitTone, when } from "./ApplicationCard";
import { FitBadge, type Match } from "./Fit";
import { Initials } from "./Initials";
import type { MoveRequest } from "./useMoveApplication";

type Stage = Schemas["Stage"];
export type PipelineView = "steps" | "board";

/** The usual next step from each stage. */
export const NEXT: Partial<Record<Stage, { stage: Stage; label: string }>> = {
  applied: { stage: "screening", label: "Screen" },
  screening: { stage: "submitted", label: "Submit to client" },
  submitted: { stage: "interview", label: "Interview" },
  interview: { stage: "offer", label: "Offer" },
  offer: { stage: "placed", label: "Placed" },
};

/** Waited long in their stage: 7 days or more (not once placed). */
const waiting = (a: Application, now: number) => a.stage !== "placed" && daysInStage(a, now) >= 7;
/** The longest waiting first. */
export const longestFirst = (now: number) => (a: Application, b: Application) => daysInStage(b, now) - daysInStage(a, now);

interface Shared {
  apps: Application[];
  picked: string[];
  onPick?: (a: Application, on: boolean) => void;
  canMove: boolean;
  busy: boolean;
  onMove: (m: MoveRequest) => void;
  upcoming: Map<string, Upcoming>;
  now: number;
  fit: Map<string, Match>;
  onShowFit: (a: Application) => void;
}

/** Clicking anywhere but a link or a control chooses the person. */
const choose = (props: Shared, a: Application) => (e: React.MouseEvent) => {
  if (!props.onPick || (e.target as HTMLElement).closest("a, button, input, select, label, form")) return;
  props.onPick(a, !props.picked.includes(a.id));
};

/** What matters about them in their stage: the interview's time, the offer's deadline, how they came. */
function Info({ a, upcoming, now }: { a: Application; upcoming?: Upcoming; now: number }) {
  if (upcoming?.interviewAt) return <span className="chip cal">{when.format(new Date(upcoming.interviewAt))}</span>;
  if (upcoming?.offerExpiresOn) return offerChip(upcoming.offerExpiresOn, now);
  return a.channel ? <span className="muted small">via {CHANNEL[a.channel]}</span> : null;
}

export function PipelineSteps(props: Shared & { stage: Stage | "all"; onStage: (s: Stage | "all") => void }) {
  const { apps, stage, now } = props;
  const shown = (stage === "all" ? [...apps] : apps.filter((a) => a.stage === stage)).sort(longestFirst(now));
  return (
    <div className="pipeline-steps">
      <div className="funnel" role="tablist" aria-label="Stages">
        <button type="button" role="tab" aria-selected={stage === "all"} className="stage-step all" onClick={() => { props.onStage("all"); }}>
          <small>All</small><b>{apps.length}</b>
        </button>
        {OPEN_STAGES.map((st) => {
          const inStage = apps.filter((a) => a.stage === st);
          const long = inStage.filter((a) => waiting(a, now)).length;
          return (
            <button key={st} type="button" role="tab" aria-selected={stage === st} className={`stage-step stage-${st}`} onClick={() => { props.onStage(st); }}>
              <small>{STAGE[st]}</small><b>{inStage.length}</b>
              {long > 0 && <span className="stage-wait">{long} waiting 7+ days</span>}
            </button>
          );
        })}
      </div>
      <div role="list" aria-label={stage === "all" ? "Everyone" : STAGE[stage]} className="step-rows">
        {shown.length === 0 && <p className="column-empty">{stage === "placed" ? "Nobody placed yet" : "Nobody here"}</p>}
        {shown.map((a) => {
          const next = NEXT[a.stage];
          const days = daysInStage(a, now);
          const match = props.fit.get(a.id);
          return (
            <div role="listitem" key={a.id} className={`step-row${props.picked.includes(a.id) ? " picked" : ""}${props.onPick ? " pickable" : ""}`} onClick={choose(props, a)}>
              <Initials name={a.candidateName} />
              <span className="who-text">
                <a href={`#/candidates/${a.candidateId}`}>{a.candidateName}</a>
                {props.picked.includes(a.id) && <span className="visually-hidden"> (chosen)</span>}
                {a.candidateTitle && <span className="who-title">{a.candidateTitle}</span>}
              </span>
              <span className="step-info">
                {stage === "all" && <span className={`badge ${a.stage}`}>{STAGE[a.stage]}</span>}
                <Info a={a} upcoming={props.upcoming.get(a.id)} now={now} />
                {match && <button type="button" className="link" aria-label={`Fit of ${a.candidateName}: why?`} onClick={() => { props.onShowFit(a); }}><FitBadge match={match} /></button>}
              </span>
              <span className={`chip${waitTone(a, days)}`}>{timeInStage(a, now)}</span>
              {props.canMove && (
                <span className="step-actions">
                  {next && <button type="button" className="primary" disabled={props.busy} onClick={() => { props.onMove({ app: a, stage: next.stage }); }}>{next.label}</button>}
                  <MoveMenu app={a} busy={props.busy} onMove={props.onMove} />
                </span>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

const FOLD = 8;

/** One person on one line of the board. */
export function BoardLine(props: Shared & { a: Application; onDragStart: (a: Application) => void }) {
  const { a, now } = props;
  const next = NEXT[a.stage];
  const days = daysInStage(a, now);
  const upcoming = props.upcoming.get(a.id);
  const match = props.fit.get(a.id);
  return (
    <div className={`board-line${props.picked.includes(a.id) ? " picked" : ""}${waiting(a, now) && days >= 14 ? " stale" : ""}${props.onPick ? " pickable" : ""}`}
      draggable={props.canMove} onDragStart={(e) => { e.dataTransfer.setData("text/plain", a.id); props.onDragStart(a); }} onClick={choose(props, a)}>
      <Initials name={a.candidateName} />
      <a href={`#/candidates/${a.candidateId}`} title={a.candidateTitle ? `${a.candidateName} · ${a.candidateTitle}` : a.candidateName}
        onKeyDown={(e) => { if (props.onPick && e.key === " ") { e.preventDefault(); props.onPick(a, !props.picked.includes(a.id)); } }}>{a.candidateName}</a>
      {props.picked.includes(a.id) && <span className="visually-hidden"> (chosen)</span>}
      <span className="line-chips">
        {match && <button type="button" className="link" aria-label={`Fit of ${a.candidateName}: why?`} onClick={() => { props.onShowFit(a); }}>
          <FitBadge match={match} /></button>}
        {(upcoming?.interviewAt || upcoming?.offerExpiresOn) && <Info a={a} upcoming={upcoming} now={now} />}
        <span className={`chip${waitTone(a, days)}`} title={`${timeInStage(a, now)} in ${STAGE[a.stage].toLowerCase()}`}>{days < 1 ? "today" : `${String(days)}d`}</span>
      </span>
      {props.canMove && next && (
        <button type="button" className="line-next" disabled={props.busy} title={`Move to ${STAGE[next.stage]}`}
          aria-label={`${next.label}: ${a.candidateName}`} onClick={() => { props.onMove({ app: a, stage: next.stage }); }}>›</button>
      )}
    </div>
  );
}

/** A column's people: the first 8, then "+N more" opens the rest. */
export function FoldedLines(props: Shared & { lines: Application[]; onDragStart: (a: Application) => void }) {
  const [open, setOpen] = useState(false);
  const shown = open ? props.lines : props.lines.slice(0, FOLD);
  return (
    <>
      {shown.map((a) => <div role="listitem" key={a.id}><BoardLine {...props} a={a} /></div>)}
      {props.lines.length > FOLD && (
        <button type="button" className="link fold-more" onClick={() => { setOpen(!open); }}>
          {open ? "Show fewer" : `+${String(props.lines.length - FOLD)} more`}
        </button>
      )}
    </>
  );
}

const VIEW_KEY = "hp.pipelineView";
/** Steps or Board, remembered in this browser. */
export function usePipelineView(): [PipelineView, (v: PipelineView) => void] {
  const [view, setView] = useState<PipelineView>(() => { try { return localStorage.getItem(VIEW_KEY) === "board" ? "board" : "steps"; } catch { return "steps"; } });
  return [view, (v) => { setView(v); try { localStorage.setItem(VIEW_KEY, v); } catch { /* not remembered */ } }];
}
