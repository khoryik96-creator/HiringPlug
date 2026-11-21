// A job's pipeline in two views (the switch above it, remembered):
// - Steps: the stages as a funnel to click, flagging who has waited long;
//   below, that stage's people, each with the button that moves them on.
// - Board: a column per stage, each person on one line (the longest waiting
//   first), the column folded after 8, a "next" button on each line.
// Clicking a person's line chooses them for the bar above (email, move), as
// the cards did.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Application, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationsKey, candidatesKey } from "../api/keys";
import { IconButton, PlusIcon } from "./ActionIcons";
import { ErrorMessage } from "./ErrorMessage";
import { type Picked, SearchPicker } from "./SearchPicker";
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
  onPickAll?: (on: boolean) => void;
  countOf: (column: Column) => number;
  waitingOf: (column: Column) => number;
  activeCount: number;
  pending: boolean;
  choosing: boolean;
  onMore?: () => void;
}) {
  const { apps, stage, now, columns } = props;
  const column = columns.find((c) => c.key === stage);
  const inColumn = (c: Column) => apps.filter((a) => columnOf(a, columns)?.key === c.key);
  const shown = (column ? inColumn(column) : [...apps]).sort(props.order);
  const total = column ? props.countOf(column) : props.activeCount;
  const allPicked = shown.length >= total && shown.length > 0 && shown.every((a) => props.picked.includes(a.id));
  const where = column ? ` in ${column.label}` : "";
  return (
    <div className="pipeline-steps">
      <div className="funnel" role="tablist" aria-label="Stages" style={{ "--cols": columns.length } as React.CSSProperties}>
        <button type="button" role="tab" aria-selected={stage === "all"} className="stage-step all" onClick={() => { props.onStage("all"); }}>
          <small>All</small><b>{props.activeCount}</b>
        </button>
        {columns.map((c) => {
          const long = props.waitingOf(c);
          return (
            <button key={c.key} type="button" role="tab" aria-selected={stage === c.key} className={`stage-step stage-${c.stage}`} onClick={() => { props.onStage(c.key); }}>
              <small>{c.label}</small><b>{props.countOf(c)}</b>
              {long > 0 && <span className="stage-wait">{long} waiting 7+ days</span>}
            </button>
          );
        })}
      </div>
      {props.onPickAll && total > 0 && (
        <div className="step-tools">
          <button type="button" className="link" disabled={props.choosing} onClick={() => { props.onPickAll?.(!allPicked); }}>
            {allPicked ? `Unchoose all ${String(total)}${where}` : `Choose all ${String(total)}${where}`}
          </button>
        </div>
      )}
      <div role="list" aria-label={column ? column.label : "Everyone"} className="step-rows">
        {shown.length === 0 && total === 0 && <p className="column-empty">{column?.stage === "placed" ? "Nobody placed yet" : "Nobody here"}</p>}
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
      {props.onMore && <button type="button" className="link" disabled={props.pending} onClick={props.onMore}>Load more</button>}
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
export function FoldedLines(props: Shared & {
  lines: Application[]; onDragStart: (a: Application) => void;
  total: number; onMore?: () => void; moreLabel: string; pending: boolean;
}) {
  const [open, setOpen] = useState(false);
  const shown = open ? props.lines : props.lines.slice(0, FOLD);
  return (
    <>
      {shown.map((a) => <div role="listitem" key={a.id}><BoardLine {...props} a={a} /></div>)}
      {props.lines.length > FOLD && (
        <button type="button" className="link fold-more" onClick={() => { setOpen(!open); }}>
          {open ? "Show fewer" : `+${String(props.total - FOLD)} more`}
        </button>
      )}
      {props.onMore && (open || props.lines.length <= FOLD) && (
        <button type="button" className="link" disabled={props.pending} onClick={props.onMore}>{props.moreLabel}</button>
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

/** Put a candidate on this job. */
export function AddCandidate({ jobId }: { jobId: string }) {
  const queryClient = useQueryClient();
  const [picked, setPicked] = useState<Picked | null>(null);
  const add = useMutation({
    mutationFn: async (candidateId: string) =>
      unwrap(await api.POST("/v1/jobs/{jobId}/applications", { params: { path: { jobId } }, body: { candidateId } })),
    onSuccess: () => {
      setPicked(null);
      void queryClient.invalidateQueries({ queryKey: applicationsKey });
    },
  });
  return (
    <div className="row">
      <SearchPicker
        value={picked}
        onChange={(p) => { setPicked(p); add.reset(); }}
        queryKey={candidatesKey}
        placeholder="Add a candidate"
        noMatch="No matching candidates."
        search={async (q) =>
          unwrap(await api.GET("/v1/candidates", { params: { query: { q, limit: 8 } } })).items.map((c) => ({ id: c.id, name: c.fullName, detail: c.currentTitle }))}
      />
      {picked && <IconButton label="Add to job" icon={<PlusIcon />} className="icon-outline" disabled={add.isPending} onClick={() => { add.mutate(picked.id); }} />}
      <ErrorMessage error={add.error} />
    </div>
  );
}

/**
 * What is coming up for candidates, by application: the next scheduled
 * interview (8 weeks ahead) and a sent offer's last day to answer. Read with
 * the user's own permissions: without one, that part is simply not shown.
 */
export function useUpcoming(permissions: Permission[], now: number): Map<string, Upcoming> {
  const interviews = useQuery({
    queryKey: ["interviews", "upcoming", Math.floor(now / 3_600_000)],
    queryFn: async () => unwrap(await api.GET("/v1/interviews", { params: { query: {
      from: new Date(now).toISOString(), to: new Date(now + 56 * 86_400_000).toISOString(),
    } } })).items,
    enabled: permissions.includes("interviews:read"),
    retry: false,
  });
  const offers = useQuery({
    queryKey: ["offers", "sent"],
    queryFn: async () => unwrap(await api.GET("/v1/offers", { params: { query: { status: "sent" } } })).items,
    enabled: permissions.includes("offers:read"),
    retry: false,
  });
  const out = new Map<string, Upcoming>();
  for (const i of interviews.data ?? []) {
    if (i.status !== "scheduled" || Date.parse(i.startsAt) < now) continue;
    const had = out.get(i.applicationId);
    if (!had?.interviewAt || i.startsAt < had.interviewAt) out.set(i.applicationId, { ...had, interviewAt: i.startsAt });
  }
  for (const o of offers.data ?? []) {
    if (o.expiresOn) out.set(o.applicationId, { ...out.get(o.applicationId), offerExpiresOn: o.expiresOn });
  }
  return out;
}
