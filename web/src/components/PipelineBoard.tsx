import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { api, ApiError, type Application, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationsKey, candidatesKey, jobApplicationsKey } from "../api/keys";
import { CLOSE_REASON, OPEN_STAGES, STAGE } from "../labels";
import { MoveChosen, MoveMenu, type Upcoming } from "./ApplicationCard";
import { ClientEmail } from "./clientEmail/ClientEmail";
import type { Shortlisted } from "./clientEmail/Shortlist";
import { type Person, SeparateEmail } from "./SeparateEmail";
import { ErrorMessage } from "./ErrorMessage";
import { hashParam } from "../hashQuery";
import { FitBadge, FitReasons, ScoreJobFit, useJobMatches } from "./Fit";
import { type Picked, SearchPicker } from "./SearchPicker";
import { FoldedLines, longestFirst, PipelineSteps, usePipelineView } from "./PipelineViews";
import { needsDetails, StagePrompt } from "./StagePrompt";
import { type MoveRequest, useMoveApplication, useMoveApplications } from "./useMoveApplication";

/**
 * A job's pipeline: a column per stage, closed applications listed below.
 * With matching, each card shows its fit score and the board can put the
 * best fits first.
 */
export function PipelineBoard({ jobId, canUpdate, permissions = [], children }: {
  jobId: string; canUpdate: boolean; permissions?: Permission[];
  /** Shown under the board (the job's possible candidates). */
  children?: ReactNode;
}) {
  const queryClient = useQueryClient();
  const list = useQuery({
    queryKey: jobApplicationsKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/applications", { params: { path: { jobId } } })),
  });
  const move = useMoveApplication();
  const [dragging, setDragging] = useState<Application | null>(null);
  const [over, setOver] = useState<Schemas["Stage"] | null>(null);
  const apps = list.data?.items ?? [];
  // Opened from a dashboard number ("?stage=screening"): that stage is shown
  // and marked for a few seconds.
  const [spotlight, setSpotlight] = useState(() => {
    const st = hashParam("stage");
    return (OPEN_STAGES as readonly string[]).includes(st) ? (st as Schemas["Stage"]) : null;
  });
  const board = useRef<HTMLDivElement>(null);
  const loaded = list.data !== undefined;
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
  const matches = useJobMatches(jobId, permissions.includes("matching:read"));
  const fit = new Map((matches.data?.items ?? []).map((m) => [m.applicationId, m]));
  const [bestFirst, setBestFirst] = useState(false);
  const [fitShown, setFitShown] = useState<Application | null>(null);
  // Candidates chosen for one email to the client, in the board's order.
  const [picked, setPicked] = useState<string[]>([]);
  const [shortlist, setShortlist] = useState<Shortlisted[] | null>(null);
  const [separately, setSeparately] = useState<Person[] | null>(null);
  const canEmail = permissions.includes("emails:send");
  const canPick = canEmail || canUpdate;
  const chosenApps = apps.filter((a) => picked.includes(a.id));
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
    const chosen = OPEN_STAGES.flatMap((st) => apps.filter((a) => a.stage === st && picked.includes(a.id)).sort(shownOrder));
    // One candidate: the email about their application, with their own fields filled in.
    if (chosen.length === 1 && chosen[0]) { setEmailing(chosen[0]); return; }
    setShortlist(chosen.map((a) => ({ candidateId: a.candidateId, applicationId: a.id, name: a.candidateName, title: a.candidateTitle, stage: a.stage })));
  };
  const [view, setView] = usePipelineView();
  // Steps opens where most people have waited 7+ days (else the first stage with anyone).
  const waitingIn = (st: Schemas["Stage"]) => st === "placed" ? 0 : active.filter((a) => a.stage === st && Date.parse(a.stageChangedAt) < now - 7 * 86_400_000).length;
  const mostWaiting = [...OPEN_STAGES].sort((x, y) => waitingIn(y) - waitingIn(x))[0];
  const firstStage = (): Schemas["Stage"] | "all" => spotlight
    ?? (mostWaiting && waitingIn(mostWaiting) > 0 ? mostWaiting : undefined)
    ?? OPEN_STAGES.find((st) => active.some((a) => a.stage === st)) ?? "all";
  const [stepStage, setStepStage] = useState<Schemas["Stage"] | "all" | null>(null);
  // Chosen once, when the pipeline loads: it does not jump when people move or the spotlight ends.
  if (loaded && stepStage === null) setStepStage(firstStage()); // set during render, once (React's adjust-state pattern)
  const shared = {
    picked, onPick: canPick ? pick : undefined, canMove: canUpdate, busy: move.isPending || moveMany.isPending, onMove: requestMove,
    upcoming, now, fit, onShowFit: setFitShown,
  };
  const shownMatch = fitShown ? fit.get(fitShown.id) : undefined;

  return (
    <section className="panel" aria-label="Pipeline">
      <div className="pipeline-head">
        <h3>Pipeline <span className="count-chip">{String(apps.length - closed.length)} active</span></h3>
        <span className="view-tabs" role="group" aria-label="View">
          <button type="button" aria-pressed={view === "steps"} onClick={() => { setView("steps"); }}>Steps</button>
          <button type="button" aria-pressed={view === "board"} onClick={() => { setView("board"); }}>Board</button>
        </span>
      </div>
      <div className="pipeline-tools">
        <div className="row">{canUpdate && <AddCandidate jobId={jobId} />}</div>
        <div className="row">
          {fit.size > 0 && (
            <select className="order-select" aria-label="Order" value={bestFirst ? "fit" : "waiting"}
              onChange={(e) => { setBestFirst(e.target.value === "fit"); }}>
              <option value="waiting">Longest waiting</option>
              <option value="fit">Best fit</option>
            </select>
          )}
          {matches.data && permissions.includes("matching:create") && <ScoreJobFit jobId={jobId} enabled={matches.data.enabled} chosen={chosenApps.filter((a) => a.stage !== "rejected" && a.stage !== "withdrawn").map((a) => a.id)} />}
        </div>
      </div>
      {separately && <SeparateEmail people={separately} onClose={() => { setSeparately(null); setPicked([]); }} />}
      {canPick && picked.length > 0 && !shortlist && !separately && !emailing && (
        <div className="pick-bar" role="status">
          <span>{picked.length} chosen</span>
          {canEmail && <>
            <button type="button" className="primary" disabled={picked.length > 10} onClick={startShortlist}>
              Email client about {picked.length} candidate{picked.length === 1 ? "" : "s"}
            </button>
            <button type="button" onClick={() => {
              setSeparately(chosenApps.map((a) => ({ candidateId: a.candidateId, name: a.candidateName })));
            }}>Email each candidate separately</button>
          </>}
          {canUpdate && <MoveChosen apps={chosenApps} busy={move.isPending || moveMany.isPending} onMove={moveChosen} />}
          <button type="button" className="link" onClick={() => { setPicked([]); }}>Clear</button>
          {canEmail && picked.length > 10 && <span className="warning small">At most 10 in one email to the client.</span>}
        </div>
      )}
      {askingNow && (askingNow.stage === "offer" || askingNow.stage === "placed") && (
        <StagePrompt key={`${askingNow.app.id}-${askingNow.stage}`} app={askingNow.app} stage={askingNow.stage} permissions={permissions}
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
            <button type="button" className="link" onClick={() => { setFitShown(null); }}>Close</button>
          </div>
          <FitReasons match={shownMatch} open />
        </section>
      )}
      <ErrorMessage error={list.error ?? moveError}>
        {stale && (
          <button type="button" className="link" onClick={() => { void queryClient.invalidateQueries({ queryKey: applicationsKey }); move.reset(); moveMany.reset(); }}>
            Reload the board
          </button>
        )}
      </ErrorMessage>
      {view === "steps" && loaded && (
        <PipelineSteps {...shared} apps={active} stage={stepStage ?? firstStage()} onStage={setStepStage} order={shownOrder}
          onPickAll={canPick ? (these, on) => {
            setPicked((ids) => (on ? [...new Set([...ids, ...these.map((a) => a.id)])] : ids.filter((id) => !these.some((a) => a.id === id))));
          } : undefined} />
      )}
      {view === "board" && <div className="board compact" ref={board}>
        {OPEN_STAGES.map((stage) => {
          const inStage = apps.filter((a) => a.stage === stage);
          const cards = inStage.sort(shownOrder);
          return (
            <div
              key={stage}
              role="list"
              aria-label={STAGE[stage]}
              className={`column stage-${stage}${over === stage ? " drop" : ""}${spotlight === stage ? " spotlight" : ""}`}
              onDragOver={(e) => { if (dragging && dragging.stage !== stage) { e.preventDefault(); setOver(stage); } }}
              onDragLeave={() => { setOver(null); }}
              onDrop={(e) => {
                e.preventDefault();
                setOver(null);
                if (dragging && dragging.stage !== stage) requestMove({ app: dragging, stage });
                setDragging(null);
              }}
            >
              <div className="column-head">
                <h4>{STAGE[stage]}</h4>
                {canPick && cards.length > 0 ? (
                  // The count chooses the column's candidates (again: unchooses them).
                  <button type="button" className="count-chip column-count" title={`Choose all in ${STAGE[stage]}`}
                    aria-label={`Choose all ${String(cards.length)} in ${STAGE[stage]}`}
                    aria-pressed={cards.every((a) => picked.includes(a.id))}
                    onClick={() => {
                      const all = cards.every((a) => picked.includes(a.id));
                      setPicked((ids) => (all ? ids.filter((id) => !cards.some((a) => a.id === id)) : [...new Set([...ids, ...cards.map((a) => a.id)])]));
                    }}>{String(cards.length)}</button>
                ) : <span className="count-chip">{String(cards.length)}</span>}
              </div>
              {cards.length === 0 && <p className="column-empty">{stage === "placed" ? "Drag here when they accept" : "Nobody here"}</p>}
              <FoldedLines {...shared} apps={cards} lines={cards} onDragStart={setDragging} />
            </div>
          );
        })}
      </div>}
      {closed.length > 0 && (
        <details className="fold">
          <summary><span className="fold-title">Rejected or withdrawn</span> <span className="count-chip">{String(closed.length)}</span></summary>
          <ul className="closed-list">
            {closed.map((a) => (
              <li key={a.id}>
                <a href={`#/candidates/${a.candidateId}`}>{a.candidateName}</a>{" "}
                <span className={`badge ${a.stage}`}>{STAGE[a.stage]}</span>{" "}
                <span className="muted">{a.reason ? CLOSE_REASON[a.reason] : ""}{a.reasonNote ? `: ${a.reasonNote}` : ""}</span>{" "}
                {canUpdate && <MoveMenu app={a} busy={move.isPending} onMove={requestMove} />}
              </li>
            ))}
          </ul>
        </details>
      )}
      {children}
    </section>
  );
}

/** Put a candidate on this job. */
function AddCandidate({ jobId }: { jobId: string }) {
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
        noMatch="No candidate matches. Add them under Candidates first."
        search={async (q) =>
          unwrap(await api.GET("/v1/candidates", { params: { query: { q, limit: 8 } } })).items.map((c) => ({ id: c.id, name: c.fullName, detail: c.currentTitle }))}
      />
      {picked && <button type="button" disabled={add.isPending} onClick={() => { add.mutate(picked.id); }}>Add to job</button>}
      <ErrorMessage error={add.error} />
    </div>
  );
}

/**
 * What is coming up for candidates, by application: the next scheduled
 * interview (8 weeks ahead) and a sent offer's last day to answer. Read with
 * the user's own permissions: without one, that part is simply not shown.
 */
function useUpcoming(permissions: Permission[], now: number): Map<string, Upcoming> {
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
