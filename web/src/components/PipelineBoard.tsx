import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Application, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationsKey, candidatesKey, jobApplicationsKey } from "../api/keys";
import { CLOSE_REASON, OPEN_STAGES, STAGE } from "../labels";
import { ApplicationCard, MoveMenu } from "./ApplicationCard";
import { ErrorMessage } from "./ErrorMessage";
import { FitBadge, FitReasons, ScoreJobFit, useJobMatches } from "./Fit";
import { type Picked, SearchPicker } from "./SearchPicker";
import { useMoveApplication } from "./useMoveApplication";

/**
 * A job's pipeline: a column per stage, closed applications listed below.
 * With matching, each card shows its fit score and the board can put the
 * best fits first.
 */
export function PipelineBoard({ jobId, canUpdate, permissions = [] }: { jobId: string; canUpdate: boolean; permissions?: Permission[] }) {
  const queryClient = useQueryClient();
  const list = useQuery({
    queryKey: jobApplicationsKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/applications", { params: { path: { jobId } } })),
  });
  const move = useMoveApplication();
  const [dragging, setDragging] = useState<Application | null>(null);
  const [over, setOver] = useState<Schemas["Stage"] | null>(null);
  const apps = list.data?.items ?? [];
  const closed = apps.filter((a) => a.stage === "rejected" || a.stage === "withdrawn");
  const stale = move.error instanceof ApiError && move.error.code === "CONFLICT_STALE_VERSION";
  const matches = useJobMatches(jobId, permissions.includes("matching:read"));
  const fit = new Map((matches.data?.items ?? []).map((m) => [m.applicationId, m]));
  const [bestFirst, setBestFirst] = useState(false);
  const [fitShown, setFitShown] = useState<Application | null>(null);
  const shownMatch = fitShown ? fit.get(fitShown.id) : undefined;
  const scoreOf = (a: Application) => fit.get(a.id)?.score ?? -1;

  return (
    <section className="panel" aria-label="Pipeline">
      <div className="row spread">
        <h3>Pipeline ({String(apps.length - closed.length)} active)</h3>
        {canUpdate && <AddCandidate jobId={jobId} />}
      </div>
      {matches.data && permissions.includes("matching:create") && (
        <ScoreJobFit jobId={jobId} enabled={matches.data.enabled} anyScored={fit.size > 0} />
      )}
      {fit.size > 0 && (
        <label className="small">
          <input type="checkbox" checked={bestFirst} onChange={(e) => { setBestFirst(e.target.checked); }} /> Best fit first
        </label>
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
      <ErrorMessage error={list.error ?? move.error}>
        {stale && (
          <button type="button" className="link" onClick={() => { void queryClient.invalidateQueries({ queryKey: applicationsKey }); move.reset(); }}>
            Reload the board
          </button>
        )}
      </ErrorMessage>
      <div className="board">
        {OPEN_STAGES.map((stage) => {
          const inStage = apps.filter((a) => a.stage === stage);
          const cards = bestFirst ? inStage.sort((a, b) => scoreOf(b) - scoreOf(a)) : inStage;
          return (
            <div
              key={stage}
              role="list"
              aria-label={STAGE[stage]}
              className={over === stage ? "column drop" : "column"}
              onDragOver={(e) => { if (dragging && dragging.stage !== stage) { e.preventDefault(); setOver(stage); } }}
              onDragLeave={() => { setOver(null); }}
              onDrop={(e) => {
                e.preventDefault();
                setOver(null);
                if (dragging && dragging.stage !== stage) move.mutate({ app: dragging, stage });
                setDragging(null);
              }}
            >
              <h4>{STAGE[stage]} ({String(cards.length)})</h4>
              {cards.map((a) => (
                <div role="listitem" key={a.id}>
                  <ApplicationCard app={a} match={fit.get(a.id)} onShowFit={setFitShown} canMove={canUpdate} busy={move.isPending} onMove={(m) => { move.mutate(m); }} onDragStart={setDragging} />
                </div>
              ))}
            </div>
          );
        })}
      </div>
      {closed.length > 0 && (
        <details>
          <summary>Rejected or withdrawn ({String(closed.length)})</summary>
          <ul className="closed-list">
            {closed.map((a) => (
              <li key={a.id}>
                <a href={`#/candidates/${a.candidateId}`}>{a.candidateName}</a>{" "}
                <span className={`badge ${a.stage}`}>{STAGE[a.stage]}</span>{" "}
                <span className="muted">{a.reason ? CLOSE_REASON[a.reason] : ""}{a.reasonNote ? `: ${a.reasonNote}` : ""}</span>{" "}
                {canUpdate && <MoveMenu app={a} busy={move.isPending} onMove={(m) => { move.mutate(m); }} />}
              </li>
            ))}
          </ul>
        </details>
      )}
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
        placeholder="Add a candidate: type a name"
        noMatch="No candidate matches. Add them under Candidates first."
        search={async (q) =>
          unwrap(await api.GET("/v1/candidates", { params: { query: { q, limit: 8 } } })).items.map((c) => ({ id: c.id, name: c.fullName, detail: c.currentTitle }))}
      />
      {picked && <button type="button" disabled={add.isPending} onClick={() => { add.mutate(picked.id); }}>Add to job</button>}
      <ErrorMessage error={add.error} />
    </div>
  );
}
