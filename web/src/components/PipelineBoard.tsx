import { BATCH_MAX, BatchFormat, type BatchPerson } from "./BatchFormat";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { ApiError, type Application, type Permission, type Schemas } from "../api/client";
import { completePipelineStage, usePipelinePages } from "./pipelinePages";
import { applicationsKey } from "../api/keys";
import { CLOSE_REASON, OPEN_STAGES, STAGE } from "../labels";
import { BoardIcon, CloseIcon, FileIcon, IconButton, LinkIcon, MailIcon, MailManyIcon, StepsIcon } from "./ActionIcons";
import { MoveChosen, MoveMenu } from "./ApplicationCard";
import { ClientEmail } from "./clientEmail/ClientEmail";
import type { Shortlisted } from "./clientEmail/Shortlist";
import { type Person, SeparateEmail } from "./SeparateEmail";
import type { LinkCandidate } from "./shortlists/ClientLinkForm";
import { ErrorMessage } from "./ErrorMessage";
import { hashParam } from "../hashQuery";
import { FitBadge, FitReasons, ScoreJobFit, useJobMatches } from "./Fit";
import { AddCandidate, FoldedLines, longestFirst, PipelineSteps, usePipelineView, useUpcoming } from "./PipelineViews";
import { needsDetails, StagePrompt } from "./StagePrompt";
import { type Column, columnOf, columnsFor, ownStages, STANDARD, useJobStages } from "./stages/columns";
import { JobStagesControl } from "./stages/JobStagesControl";
import { type MoveRequest, useMoveApplication, useMoveApplications } from "./useMoveApplication";

/**
 * A job's pipeline: a column per stage, closed applications listed below.
 * With matching, each card shows its fit score and the board can put the
 * best fits first.
 */
export function PipelineBoard({ jobId, canUpdate, permissions = [], onClientLink, children }: {
  jobId: string; canUpdate: boolean; permissions?: Permission[];
  /** Starts a client shortlist link for the chosen candidates (with shortlists:share). */
  onClientLink?: (people: LinkCandidate[]) => void;
  /** Shown under the board (the job's possible candidates). */
  children?: ReactNode;
}) {
  const queryClient = useQueryClient();
  const list = usePipelinePages(jobId);
  const move = useMoveApplication();
  const stages = useJobStages(jobId);
  const [dragging, setDragging] = useState<Application | null>(null);
  const [over, setOver] = useState<string | null>(null);
  const apps = list.items;
  // Opened from a dashboard number ("?stage=screening"): that stage is shown
  // and marked for a few seconds.
  const [spotlight, setSpotlight] = useState(() => {
    const st = hashParam("stage");
    return (OPEN_STAGES as readonly string[]).includes(st) ? (st as Schemas["Stage"]) : null;
  });
  const board = useRef<HTMLDivElement>(null);
  const loaded = list.loaded;
  useEffect(() => {
    if (!spotlight || !loaded) return;
    const col = board.current?.querySelector<HTMLElement>(`.stage-${spotlight}`);
    if (col && typeof col.scrollIntoView === "function") col.scrollIntoView({ block: "nearest", inline: "center" });
    const t = window.setTimeout(() => { setSpotlight(null); }, 4000);
    return () => { window.clearTimeout(t); };
  }, [spotlight, loaded]);
  const [now] = useState(() => Date.now());
  const upcoming = useUpcoming(permissions, now);
  const closed = apps.filter((a) => a.stage === "rejected" || a.stage === "withdrawn");
  const active = apps.filter((a) => a.stage !== "rejected" && a.stage !== "withdrawn");
  // The job's own stages (else the standard ones), and a column for anyone in a kind it no longer has.
  const columns = columnsFor(stages.data?.steps ?? STANDARD, list.counts ?? active, ownStages(stages.data?.source));
  const inColumn = (key: string) => active.filter((a) => columnOf(a, columns)?.key === key);
  const grouped = (column: Column) => list.counts?.filter((row) => columnOf(row, columns)?.key === column.key);
  const countOf = (column: Column) => grouped(column)?.reduce((n, row) => n + row.count, 0) ?? inColumn(column.key).length;
  const waitingOf = (column: Column) => grouped(column)?.reduce((n, row) => n + row.waiting, 0)
    ?? inColumn(column.key).filter((a) => a.stage !== "placed" && Date.parse(a.stageChangedAt) < now - 7 * 86_400_000).length;
  const activeCount = columns.reduce((n, column) => n + countOf(column), 0);
  const closedCount = list.counts?.filter((row) => row.stage === "rejected" || row.stage === "withdrawn").reduce((n, row) => n + row.count, 0) ?? closed.length;
  const matches = useJobMatches(jobId, permissions.includes("matching:read"));
  const fit = new Map((matches.data?.items ?? []).map((m) => [m.applicationId, m]));
  const [bestFirst, setBestFirst] = useState(false);
  const [fitShown, setFitShown] = useState<Application | null>(null);
  // Candidates chosen for one email to the client, in the board's order.
  const [picked, setPicked] = useState<string[]>([]);
  const [shortlist, setShortlist] = useState<Shortlisted[] | null>(null);
  const [separately, setSeparately] = useState<Person[] | null>(null);
  const canEmail = permissions.includes("emails:send");
  const canFormat = permissions.includes("cvformats:create");
  const [formatting, setFormatting] = useState<BatchPerson[] | null>(null);
  const canPick = canEmail || canUpdate || canFormat || Boolean(onClientLink);
  const chosenApps = apps.filter((a) => picked.includes(a.id));
  const setSelected = (selected: Application[], on: boolean) => {
    const ids = new Set(selected.map((a) => a.id));
    setPicked((before) => on ? [...new Set([...before, ...ids])] : before.filter((id) => !ids.has(id)));
  };
  const chooseUnloaded = useMutation({
    mutationFn: async (column?: Column) => {
      const scope = column?.stage ?? "all";
      const result = await queryClient.query(completePipelineStage(jobId, scope));
      list.reveal(scope);
      return result.items.filter((a) => column ? columnOf(a, columns)?.key === column.key : a.stage !== "rejected" && a.stage !== "withdrawn");
    },
    onSuccess: (selected) => { setSelected(selected, true); },
  });
  function chooseAll(on: boolean, column?: Column) {
    const current = column ? inColumn(column.key) : active;
    if (on && current.length < (column ? countOf(column) : activeCount)) chooseUnloaded.mutate(column);
    else setSelected(current, on);
  }
  // The chosen cards move together; those that moved are unticked, a stale one reports itself.
  const moveMany = useMoveApplications((ids) => { setPicked((cur) => cur.filter((id) => !ids.includes(id))); });
  // Offer and Placed ask for the offer or the placement report first, one candidate at a time.
  const [asking, setAsking] = useState<MoveRequest[]>([]);
  const requestMove = (m: MoveRequest) => {
    if (needsDetails(m, permissions)) setAsking((q) => [...q, m]);
    else move.mutate(m);
  };
  const moveChosen = (to: Omit<MoveRequest, "app">) => {
    const requests = chosenApps.filter((a) => a.stage !== to.stage).map((app) => ({ app, ...to }));
    const ask = requests.filter((m) => needsDetails(m, permissions));
    if (ask.length > 0) {
      setAsking((q) => [...q, ...ask]);
      setPicked((cur) => cur.filter((id) => !ask.some((m) => m.app.id === id)));
    }
    const plain = requests.filter((m) => !needsDetails(m, permissions));
    if (plain.length > 0) moveMany.mutate(plain);
  };
  const askingNow = asking[0];
  const [emailing, setEmailing] = useState<Application | null>(null);
  const moveError = move.error ?? moveMany.error;
  const stale = moveError instanceof ApiError && moveError.code === "CONFLICT_STALE_VERSION";
  const pick = (a: Application, on: boolean) => { setPicked((cur) => (on ? [...cur, a.id] : cur.filter((id) => id !== a.id))); };
  const scoreOf = (a: Application) => fit.get(a.id)?.score ?? -1;
  // The order people are shown in, in both views: best fit first when asked, else the longest waiting.
  const shownOrder = (a: Application, b: Application) => (bestFirst ? scoreOf(b) - scoreOf(a) : longestFirst(now)(a, b));
  const startShortlist = () => {
    // In the order they are shown: stage by stage, as ordered in each.
    const chosen = columns.flatMap((c) => inColumn(c.key).filter((a) => picked.includes(a.id)).sort(shownOrder));
    // One candidate: the email about their application, with their own fields filled in.
    if (chosen.length === 1 && chosen[0]) { setEmailing(chosen[0]); return; }
    setShortlist(chosen.map((a) => ({ candidateId: a.candidateId, applicationId: a.id, name: a.candidateName, title: a.candidateTitle, stage: a.stage })));
  };
  const [view, setView] = usePipelineView();
  // Steps opens where most people have waited 7+ days (else the first stage with anyone).
  const mostWaiting = [...columns].sort((x, y) => waitingOf(y) - waitingOf(x))[0];
  const firstStage = (): string => (spotlight ? columns.find((c) => c.stage === spotlight)?.key : undefined)
    ?? (mostWaiting && waitingOf(mostWaiting) > 0 ? mostWaiting.key : undefined)
    ?? columns.find((c) => countOf(c) > 0)?.key ?? "all";
  const [stepStage, setStepStage] = useState<string | null>(null);
  // Chosen once, when the pipeline loads: it does not jump when people move or the spotlight ends.
  if (loaded && !stages.isPending && stepStage === null) setStepStage(firstStage()); // set during render, once (React's adjust-state pattern)
  const selectedColumn = columns.find((c) => c.key === (stepStage ?? firstStage()));
  const selectedScope = selectedColumn?.stage ?? "all";
  const needsFirstPage = selectedColumn !== undefined && inColumn(selectedColumn.key).length === 0 && countOf(selectedColumn) > 0;
  useEffect(() => {
    if (view === "steps" && loaded && needsFirstPage && !list.requested(selectedScope)) list.load(selectedScope);
  }, [view, loaded, needsFirstPage, selectedScope, list]);
  const loadBestFit = useMutation({
    mutationFn: async (scope: Parameters<typeof completePipelineStage>[1]) => {
      await queryClient.query(completePipelineStage(jobId, scope));
      list.reveal(scope);
    },
  });
  const sortScope = view === "steps" ? selectedScope : "all";
  const sortCount = view === "steps" && selectedColumn ? countOf(selectedColumn) : activeCount;
  const sortLoaded = view === "steps" && selectedColumn ? inColumn(selectedColumn.key).length : active.length;
  useEffect(() => {
    if (bestFirst && sortLoaded < sortCount && loadBestFit.isIdle) loadBestFit.mutate(sortScope);
  }, [bestFirst, sortScope, sortCount, sortLoaded, loadBestFit]);
  const canLoadMore = (column?: Column) => column
    ? inColumn(column.key).length < countOf(column) && (!list.requested(column.stage) || list.more(column.stage))
    : list.more("all");
  const shared = {
    picked, onPick: canPick ? pick : undefined, canMove: canUpdate, busy: move.isPending || moveMany.isPending, onMove: requestMove,
    upcoming, now, fit, onShowFit: setFitShown, columns,
  };
  const shownMatch = fitShown ? fit.get(fitShown.id) : undefined;

  return (
    <section className="panel" aria-label="Pipeline">
      <div className="pipeline-head">
        <h3>Pipeline <span className="count-chip">{String(activeCount)} active</span></h3>
        {stages.data && <JobStagesControl jobId={jobId} stages={stages.data} canChange={canUpdate} />}
        <span className="view-tabs" role="group" aria-label="View">
          <IconButton label="Steps" icon={<StepsIcon />} pressed={view === "steps"} onClick={() => { loadBestFit.reset(); setView("steps"); }} />
          <IconButton label="Board" icon={<BoardIcon />} pressed={view === "board"} onClick={() => { loadBestFit.reset(); setView("board"); }} />
        </span>
      </div>
      <div className="pipeline-tools">
        <div className="row">{canUpdate && <AddCandidate jobId={jobId} />}</div>
        <div className="row">
          {fit.size > 0 && (
            <select className="order-select" aria-label="Order" value={bestFirst ? "fit" : "waiting"}
              onChange={(e) => { loadBestFit.reset(); setBestFirst(e.target.value === "fit"); }}>
              <option value="waiting">Longest waiting</option>
              <option value="fit">Best fit</option>
            </select>
          )}
          {loadBestFit.isPending && <span className="muted small" role="status">Loading candidates…</span>}
          {matches.data && permissions.includes("matching:create") && <ScoreJobFit jobId={jobId} enabled={matches.data.enabled} chosen={chosenApps.filter((a) => a.stage !== "rejected" && a.stage !== "withdrawn").map((a) => a.id)} />}
        </div>
      </div>
      {separately && <SeparateEmail people={separately} onClose={() => { setSeparately(null); setPicked([]); }} />}
      {formatting && <BatchFormat people={formatting} onClose={() => { setFormatting(null); setPicked([]); }} />}
      {canPick && picked.length > 0 && !shortlist && !separately && !emailing && !formatting && (
        <div className="pick-bar" role="status">
          <span>{picked.length} chosen</span>
          {canEmail && <>
            <IconButton label={`Email client about ${String(picked.length)} candidate${picked.length === 1 ? "" : "s"}`} tip="Email the client about them"
              icon={<MailIcon />} className="icon-outline" disabled={picked.length > 10} onClick={startShortlist} />
            <IconButton label="Email each candidate separately" tip="Email each candidate separately" icon={<MailManyIcon />} className="icon-outline"
              onClick={() => { setSeparately(chosenApps.map((a) => ({ candidateId: a.candidateId, name: a.candidateName }))); }} />
          </>}
          {onClientLink && (
            <IconButton label={`Client link for ${String(picked.length)}`} tip="Client link: the client approves or rejects them" icon={<LinkIcon />}
              className="icon-outline" disabled={chosenApps.some((a) => a.stage === "rejected" || a.stage === "withdrawn") || picked.length > 20}
              onClick={() => {
                onClientLink(chosenApps.map((a) => ({ applicationId: a.id, candidateId: a.candidateId, name: a.candidateName, stage: a.stage })));
                setPicked([]);
              }} />
          )}
          {canFormat && (
            <IconButton label={`Format CVs for ${String(picked.length)}`} tip="Format their CVs CV Studio's way" icon={<FileIcon />} className="icon-outline"
              disabled={picked.length > BATCH_MAX}
              onClick={() => { setFormatting(chosenApps.map((a) => ({ candidateId: a.candidateId, name: a.candidateName }))); }} />
          )}
          {canUpdate && <MoveChosen apps={chosenApps} busy={move.isPending || moveMany.isPending} onMove={moveChosen} columns={columns} />}
          <IconButton label="Clear" tip="Clear selection" icon={<CloseIcon />} onClick={() => { setPicked([]); }} />
          {canEmail && picked.length > 10 && <span className="warning small">At most 10 in one email to the client.</span>}
        </div>
      )}
      {askingNow && (askingNow.stage === "offer" || askingNow.stage === "placed") && (
        <StagePrompt key={`${askingNow.app.id}-${askingNow.stage}`} app={askingNow.app} stage={askingNow.stage} step={askingNow.step} permissions={permissions}
          onMove={(m) => { move.mutate(m); }} onClose={() => { setAsking((q) => q.slice(1)); }} />
      )}
      {emailing && (
        <ClientEmail key={emailing.id} candidateId={emailing.candidateId} applicationId={emailing.id} permissions={permissions}
          onClose={() => { setEmailing(null); setPicked([]); }} />
      )}
      {shortlist && (
        <ClientEmail key={shortlist.map((p) => p.candidateId).join()} shortlist={shortlist} jobId={jobId} permissions={permissions}
          onClose={() => { setShortlist(null); setPicked([]); }} />
      )}
      {fitShown && shownMatch && (
        <section className="fit-panel" aria-label={`Fit of ${fitShown.candidateName}`}>
          <div className="row spread">
            <h4>{fitShown.candidateName} <FitBadge match={shownMatch} /></h4>
            <IconButton label="Close" tip="Close fit details" icon={<CloseIcon />} onClick={() => { setFitShown(null); }} />
          </div>
          <FitReasons match={shownMatch} open />
        </section>
      )}
      <ErrorMessage error={list.error ?? moveError ?? chooseUnloaded.error ?? loadBestFit.error}>
        {stale && (
          <button type="button" className="link" onClick={() => { void queryClient.invalidateQueries({ queryKey: applicationsKey }); move.reset(); moveMany.reset(); }}>
            Reload the board
          </button>
        )}
      </ErrorMessage>
      {view === "steps" && loaded && (
        <PipelineSteps {...shared} apps={active} stage={stepStage ?? firstStage()} onStage={(stage) => { loadBestFit.reset(); setStepStage(stage); }} order={shownOrder}
          countOf={countOf} waitingOf={waitingOf} activeCount={activeCount} choosing={chooseUnloaded.isPending}
          pending={list.pending(selectedScope)} onMore={canLoadMore(selectedColumn) ? () => { list.load(selectedScope); } : undefined}
          onPickAll={canPick ? (on) => { chooseAll(on, selectedColumn); } : undefined} />
      )}
      {view === "board" && <div className="board compact" ref={board} style={{ "--cols": columns.length, ...(columns.length > 6 ? { "--col-min": "10.5rem" } : {}) } as React.CSSProperties}>
        {columns.map((col) => {
          const cards = inColumn(col.key).sort(shownOrder);
          const count = countOf(col);
          const all = cards.length >= count && cards.every((a) => picked.includes(a.id));
          const elsewhere = (a: Application) => columnOf(a, columns)?.key !== col.key;
          return (
            <div
              key={col.key}
              role="list"
              aria-label={col.label}
              className={`column stage-${col.stage}${over === col.key ? " drop" : ""}${spotlight === col.stage && columns.find((c) => c.stage === spotlight) === col ? " spotlight" : ""}`}
              onDragOver={(e) => { if (dragging && elsewhere(dragging)) { e.preventDefault(); setOver(col.key); } }}
              onDragLeave={() => { setOver(null); }}
              onDrop={(e) => {
                e.preventDefault();
                setOver(null);
                if (dragging && elsewhere(dragging)) requestMove({ app: dragging, stage: col.stage, step: col.step });
                setDragging(null);
              }}
            >
              <div className="column-head">
                <h4>{col.label}</h4>
                {canPick && count > 0 ? (
                  // The count chooses the column's candidates (again: unchooses them).
                  <button type="button" className="count-chip column-count" title={`Choose all in ${col.label}`}
                    aria-label={`Choose all ${String(count)} in ${col.label}`}
                    aria-pressed={all} disabled={chooseUnloaded.isPending}
                    onClick={() => { chooseAll(!all, col); }}>{String(count)}</button>
                ) : <span className="count-chip">{String(count)}</span>}
              </div>
              <FoldedLines {...shared} apps={cards} lines={cards} onDragStart={setDragging} total={count}
                onMore={canLoadMore(col) ? () => { list.load(col.stage); } : undefined}
                moreLabel={`Load more ${col.label}`} pending={list.pending(col.stage)} />
            </div>
          );
        })}
      </div>}
      {closedCount > 0 && (
        <details className="fold">
          <summary><span className="fold-title">Rejected or withdrawn</span> <span className="count-chip">{String(closedCount)}</span></summary>
          <ul className="closed-list">
            {closed.map((a) => (
              <li key={a.id}>
                <a href={`#/candidates/${a.candidateId}`}>{a.candidateName}</a>{" "}
                <span className={`badge ${a.stage}`}>{STAGE[a.stage]}</span>{" "}
                <span className="muted">{a.reason ? CLOSE_REASON[a.reason] : ""}{a.reasonNote ? `: ${a.reasonNote}` : ""}</span>{" "}
                {canUpdate && <MoveMenu app={a} busy={move.isPending} onMove={requestMove} columns={columns} />}
              </li>
            ))}
          </ul>

          {(["rejected", "withdrawn"] as const).map((stage) => {
            const total = list.counts?.filter((row) => row.stage === stage).reduce((n, row) => n + row.count, 0) ?? 0;
            return closed.filter((a) => a.stage === stage).length < total && (!list.requested(stage) || list.more(stage))
              ? <button type="button" className="link" key={stage} disabled={list.pending(stage)} onClick={() => { list.load(stage); }}>Load more {STAGE[stage]}</button>
              : null;
          })}
        </details>
      )}
      {children}
    </section>
  );
}
