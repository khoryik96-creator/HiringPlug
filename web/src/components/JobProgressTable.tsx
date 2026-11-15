// The jobs list: each job with where its candidates are (a bar per stage),
// how long it has been open, its owner, and why it needs attention.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type JobOrderSummary, type Schemas, unwrap } from "../api/client";
import { jobsKey } from "../api/keys";
import { JOB_STATUS, OPEN_STAGES, STAGE } from "../labels";
import { Initials } from "./Initials";

type Progress = Schemas["JobProgress"];

const DAY = 86_400_000;
const days = (iso: string, now: number) => Math.max(0, Math.floor((now - Date.parse(iso)) / DAY));
const total = (p: Progress) => OPEN_STAGES.reduce((n, s) => n + p.stageCounts[s], 0);

/** Why an open job needs attention, in words; null when it does not. */
export function attention(job: JobOrderSummary, p: Progress | undefined, now = Date.now()): string | null {
  if (job.status !== "open" || !p) return null;
  const open = days(job.createdAt, now);
  if (total(p) === 0) return open >= 14 ? `No candidates in ${String(Math.floor(open / 7))} weeks` : null;
  if (p.waiting > 0) return `${String(p.waiting)} waiting over 7 days`;
  if (p.lastMovedAt && days(p.lastMovedAt, now) >= 7) return `Nothing moved in ${String(days(p.lastMovedAt, now))} days`;
  return null;
}

/** The progress of these jobs (candidates per stage, last movement), by job. */
export function useJobProgress(ids: string[]) {
  return useQuery({
    queryKey: [...jobsKey, "progress", ids],
    queryFn: async () => unwrap(await api.GET("/v1/pipeline/jobs", { params: { query: { jobId: ids } } })).items,
    enabled: ids.length > 0,
    retry: false, // without pipeline:read the list simply shows no bars
    select: (items) => new Map(items.map((p) => [p.jobId, p])),
  });
}

const SHADES: Record<string, string> = {
  applied: "#c4b5fd", screening: "#a78bfa", submitted: "#7352d8", interview: "#2563eb", offer: "#e8a317", placed: "#16a34a",
};
/** Where a job's candidates are: one bar, a segment per stage. */
function PipelineBar({ p }: { p: Progress }) {
  const n = total(p);
  const parts = OPEN_STAGES.filter((s) => p.stageCounts[s] > 0);
  return (
    <div>
      <div className="pipeline-bar" aria-hidden="true">
        {parts.map((s) => <i key={s} style={{ width: `${String((p.stageCounts[s] / n) * 100)}%`, background: SHADES[s] }} />)}
      </div>
      <div className="pipeline-text">
        {n === 0 ? "No candidates yet" : parts.map((s) => `${String(p.stageCounts[s])} ${STAGE[s].toLowerCase()}`).join(" · ")}
      </div>
    </div>
  );
}

export function JobProgressTable({ jobs, progress, empty }: {
  jobs: JobOrderSummary[]; progress?: Map<string, Progress>; empty: string;
}) {
  const [now] = useState(() => Date.now());
  return (
    <table className="job-progress">
      <thead>
        <tr><th>Job</th><th>Pipeline</th><th>Open</th><th>Owner</th><th><span className="visually-hidden">Needs attention</span></th></tr>
      </thead>
      <tbody>
        {jobs.map((j) => {
          const p = progress?.get(j.id);
          const why = attention(j, p, now);
          return (
            <tr key={j.id} className={why ? "needs-attention" : undefined}>
              <td>
                <a href={`#/jobs/${j.id}`} className="job-title">{j.title}</a>
                {j.status !== "open" && <> <span className={`badge ${j.status}`}>{JOB_STATUS[j.status]}</span></>}
                <div className="muted small">
                  <a href={`#/companies/${j.companyId}`} className="quiet-link">{j.companyName}</a>{j.location ? ` · ${j.location}` : ""}
                </div>
              </td>
              <td>{p ? <PipelineBar p={p} /> : null}</td>
              <td className="nowrap">{days(j.createdAt, now) === 1 ? "1 day" : `${String(days(j.createdAt, now))} days`}</td>
              <td>{j.ownerName ? <Initials name={j.ownerName} /> : <span className="muted">—</span>}</td>
              <td className="attention-why">{why}</td>
            </tr>
          );
        })}
        {jobs.length === 0 && <tr><td colSpan={5} className="muted">{empty}</td></tr>}
      </tbody>
    </table>
  );
}
