// A job's pipeline in two views (the switch above it, remembered):
// - Steps: the stages as a funnel to click, flagging who has waited long;
//   below, that stage's people, each with the button that moves them on.
// - Board: a column per stage, each person on one line (the longest waiting
//   first), the column folded after 8, a "next" button on each line.
// Clicking a person's line chooses them for the bar above (email, move), as
// the cards did.
import { useState } from "react";

import type { Application, Schemas } from "../api/client";
import { CHANNEL, STAGE } from "../labels";
import { daysInStage, MoveMenu, offerChip, timeInStage, type Upcoming, waitTone, when } from "./ApplicationCard";
import { FitBadge, type Match } from "./Fit";
import { Initials } from "./Initials";
import { type Column, columnOf, nextColumn } from "./stages/columns";
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
  /** The job's stages, as columns (see stages/columns.ts). */
  columns: Column[];
}

/** Their usual next step: the next of the job's stages, named as the standard step when it is one. */
function nextStep(a: Application, columns: Column[]): { to: Column; label: string } | undefined {
  const to = nextColumn(a, columns);
  if (!to) return undefined;
  const usual = NEXT[a.stage];
  return { to, label: usual && usual.stage === to.stage && to.label === STAGE[to.stage] ? usual.label : to.label };
}

/** Clicking anywhere but a control (their name included) chooses the person. */
const choose = (props: Shared, a: Application) => (e: React.MouseEvent) => {
  const target = e.target as HTMLElement;
  if (!props.onPick || target.closest("a:not(.pick-name), button, input, select, label, form")) return;
  if (target.closest("a.pick-name") && opens(e)) return; // a tap, Enter or a new tab: opening them
  props.onPick(a, !props.picked.includes(a.id));
};

const opensElsewhere = (e: React.MouseEvent) => e.ctrlKey || e.metaKey || e.shiftKey || e.button !== 0;
/** A tap (touch, pen) or a click with no mouse (Enter, a screen reader) opens the profile: only a mouse click chooses. */
const opens = (e: React.MouseEvent) => {
  const pointer = (e.nativeEvent as PointerEvent).pointerType;
  return e.detail === 0 || pointer === "touch" || pointer === "pen" || opensElsewhere(e);
};

/**
 * Their name. Where people can be chosen, a mouse click chooses them (the
 * line's own click) and a double-click opens their profile; a tap on a phone,
 * Enter, a screen reader, Ctrl/Cmd-click and a middle click open it (on a
 * phone the rest of the line chooses). Elsewhere it is a plain link.
 */
function PickName({ props, a, title }: { props: Shared; a: Application; title?: string }) {
  const href = `#/candidates/${a.candidateId}`;
  if (!props.onPick) return <a href={href} title={title}>{a.candidateName}</a>;
  return (
    <a href={href} className="pick-name" title={`${title ? `${title}\n` : ""}Click to choose · double-click to open`}
      onClick={(e) => { if (!opens(e)) e.preventDefault(); }}
      onDoubleClick={() => { window.location.hash = href; }}
      onKeyDown={(e) => { if (e.key === " ") { e.preventDefault(); props.onPick?.(a, !props.picked.includes(a.id)); } }}>{a.candidateName}</a>
  );
}

/** What matters about them in their stage: the interview's time, the offer's deadline, how they came. */
function Info({ a, upcoming, now }: { a: Application; upcoming?: Upcoming; now: number }) {
  if (upcoming?.interviewAt) return <span className="chip cal">{when.format(new Date(upcoming.interviewAt))}</span>;
  if (upcoming?.offerExpiresOn) return offerChip(upcoming.offerExpiresOn, now);
  return a.channel ? <span className="muted small">via {CHANNEL[a.channel]}</span> : null;
}

export function PipelineSteps(props: Shared & {
  /** The column shown (its key), or everyone. */
  stage: string; onStage: (key: string) => void; order: (a: Application, b: Application) => number;
  /** Choose (or, all chosen, unchoose) everyone shown. */
  onPickAll?: (apps: Application[], on: boolean) => void;
}) {
  const { apps, stage, now, columns } = props;
  const column = columns.find((c) => c.key === stage);
  const inColumn = (c: Column) => apps.filter((a) => columnOf(a, columns)?.key === c.key);
  const shown = (column ? inColumn(column) : [...apps]).sort(props.order);
  const allPicked = shown.length > 0 && shown.every((a) => props.picked.includes(a.id));
  const where = column ? ` in ${column.label}` : "";
  return (
    <div className="pipeline-steps">
      <div className="funnel" role="tablist" aria-label="Stages" style={{ "--cols": columns.length } as React.CSSProperties}>
        <button type="button" role="tab" aria-selected={stage === "all"} className="stage-step all" onClick={() => { props.onStage("all"); }}>
          <small>All</small><b>{apps.length}</b>
        </button>
        {columns.map((c) => {
          const inStage = inColumn(c);
          const long = inStage.filter((a) => waiting(a, now)).length;
          return (
            <button key={c.key} type="button" role="tab" aria-selected={stage === c.key} className={`stage-step stage-${c.stage}`} onClick={() => { props.onStage(c.key); }}>
              <small>{c.label}</small><b>{inStage.length}</b>
              {long > 0 && <span className="stage-wait">{long} waiting 7+ days</span>}
            </button>
          );
        })}
      </div>
      {props.onPickAll && shown.length > 0 && (
        <div className="step-tools">
          <button type="button" className="link" onClick={() => { props.onPickAll?.(shown, !allPicked); }}>
            {allPicked ? `Unchoose all ${String(shown.length)}${where}` : `Choose all ${String(shown.length)}${where}`}
          </button>
        </div>
      )}
      <div role="list" aria-label={column ? column.label : "Everyone"} className="step-rows">
        {shown.length === 0 && <p className="column-empty">{column?.stage === "placed" ? "Nobody placed yet" : "Nobody here"}</p>}
        {shown.map((a) => {
          const next = nextStep(a, columns);
          const days = daysInStage(a, now);
          const match = props.fit.get(a.id);
          return (
            <div role="listitem" key={a.id} className={`step-row${props.picked.includes(a.id) ? " picked" : ""}${props.onPick ? " pickable" : ""}`} onClick={choose(props, a)}>
              <Initials name={a.candidateName} status={a.candidateAvailability} />
              <span className="who-text">
                <PickName props={props} a={a} />
                {props.picked.includes(a.id) && <span className="visually-hidden"> (chosen)</span>}
                {a.candidateTitle && <span className="who-title">{a.candidateTitle}</span>}
              </span>
              <span className="step-info">
                {!column && <span className={`badge ${a.stage}`}>{columnOf(a, columns)?.label ?? STAGE[a.stage]}</span>}
                <Info a={a} upcoming={props.upcoming.get(a.id)} now={now} />
                {match && <button type="button" className="link" aria-label={`Fit of ${a.candidateName}: why?`} onClick={() => { props.onShowFit(a); }}><FitBadge match={match} /></button>}
              </span>
              <span className={`chip${waitTone(a, days)}`}>{timeInStage(a, now)}</span>
              {props.canMove && (
                <span className="step-actions">
                  {next && <button type="button" className="primary" disabled={props.busy} onClick={() => { props.onMove({ app: a, stage: next.to.stage, step: next.to.step }); }}>{next.label}</button>}
                  <MoveMenu app={a} busy={props.busy} onMove={props.onMove} columns={columns} />
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
  const next = nextStep(a, props.columns);
  const here = columnOf(a, props.columns);
  const days = daysInStage(a, now);
  const upcoming = props.upcoming.get(a.id);
  const match = props.fit.get(a.id);
  return (
    <div className={`board-line${props.picked.includes(a.id) ? " picked" : ""}${waiting(a, now) && days >= 14 ? " stale" : ""}${props.onPick ? " pickable" : ""}${props.canMove && next ? " movable" : ""}`}
      draggable={props.canMove} onDragStart={(e) => { e.dataTransfer.setData("text/plain", a.id); props.onDragStart(a); }} onClick={choose(props, a)}>
      <Initials name={a.candidateName} status={a.candidateAvailability} />
      <PickName props={props} a={a} title={a.candidateTitle ? `${a.candidateName} · ${a.candidateTitle}` : a.candidateName} />
      {props.picked.includes(a.id) && <span className="visually-hidden"> (chosen)</span>}
      <span className="line-chips">
        {match && <button type="button" className="link" aria-label={`Fit of ${a.candidateName}: why?`} onClick={() => { props.onShowFit(a); }}>
          <FitBadge match={match} compact /></button>}
        {(upcoming?.interviewAt || upcoming?.offerExpiresOn) && <Info a={a} upcoming={upcoming} now={now} />}
        <span className={`chip${waitTone(a, days)}`} title={`${timeInStage(a, now)} in ${(here?.label ?? STAGE[a.stage]).toLowerCase()}`}>{days < 1 ? "today" : `${String(days)}d`}</span>
      </span>
      {props.canMove && next && (
        <button type="button" className="line-next" disabled={props.busy} title={`Move to ${next.to.label}`}
          aria-label={`${next.label}: ${a.candidateName}`} onClick={() => { props.onMove({ app: a, stage: next.to.stage, step: next.to.step }); }}>›</button>
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
