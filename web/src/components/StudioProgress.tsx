// CV Studio formatting of one candidate's CV, one run at a time: the button
// shows it (orange while it runs, green "Done" once saved), and a bar with
// the seconds elapsed takes the place of a history.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { studioCvsKey } from "./FormatCv";

type Run = Schemas["StudioCv"];

const LOST = 15 * 60_000; // the server takes a run queued this long as lost, too
const TYPICAL = 60; // seconds a run usually takes

/** Now, ticking every second while active. */
export function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const t = setInterval(() => { setNow(Date.now()); }, 1000);
    return () => { clearInterval(t); };
  }, [active]);
  return now;
}

export interface StudioRun {
  /** The run being made now. */
  running?: Run;
  /** The run seen running here that has finished, until acknowledged. */
  finished?: Run;
  /** Seconds since the run shown started (frozen once it finished). */
  seconds: number;
  acknowledge: () => void;
}

export function useStudioRun(candidateId: string, enabled: boolean): StudioRun {
  const queryClient = useQueryClient();
  const [watched, setWatched] = useState<string | null>(null);
  const runs = useQuery({
    queryKey: studioCvsKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/studio-cvs", { params: { path: { candidateId } } })).items,
    enabled,
    refetchInterval: (q) => (q.state.data?.[0]?.status === "queued" ? 2000 : false),
  });
  const latest = runs.data?.[0];
  const fetchedAt = runs.dataUpdatedAt;
  const running = latest?.status === "queued" && fetchedAt - Date.parse(latest.createdAt) < LOST ? latest : undefined;
  if (running && running.id !== watched) setWatched(running.id);
  const finished = watched ? runs.data?.find((r) => r.id === watched && r.status !== "queued") : undefined;
  const now = useNow(running !== undefined);
  const shown = running ?? finished;
  // A run just finished: its CV joins the documents.
  const finishedId = finished?.id;
  useEffect(() => {
    if (finishedId) void queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
  }, [finishedId, candidateId, queryClient]);
  return {
    running, finished,
    seconds: shown ? Math.max(0, Math.round((now - Date.parse(shown.createdAt)) / 1000)) : 0,
    acknowledge: () => { setWatched(null); },
  };
}

/** The bar: filling while the CV is formatted (an estimate: about a minute), full once done. */
export function StudioProgress({ run }: { run: StudioRun }) {
  const r = run.running ?? run.finished;
  if (!r) return null;
  const s = run.seconds;
  const state = run.running ? "running" : r.status;
  const pct = run.running ? Math.min(95, 95 * (1 - Math.exp(-s / (TYPICAL / 2)))) : 100;
  return (
    <div className={`studio-progress ${state}`} role="status" aria-label="CV formatting">
      <div className="studio-line">
        <span className="studio-label">
          {run.running ? `Formatting${r.blind ? " (blind)" : ""}…` : r.status === "done" ? <>Saved <strong>{r.fileName}</strong></> : "Not formatted"}
        </span>
        <span className="studio-bar" aria-hidden="true"><i style={{ width: `${String(pct)}%` }} /></span>
        <span className="studio-timer" aria-label={`${String(s)} seconds`}>{s}s</span>
      </div>
      {r.warning && <small className="warning">Compare with the CV before sending: {r.warning}</small>}
      {r.status === "failed" && <small className="warning" role="alert">{r.error}</small>}
    </div>
  );
}
