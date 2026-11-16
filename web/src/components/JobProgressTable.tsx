// The jobs list as a heat map: each job's candidates per stage, how long it
// has been open, when something last happened, what is next, and its owner.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type JobOrderSummary, type Schemas, unwrap } from "../api/client";
import { jobsKey } from "../api/keys";
import { JOB_STATUS, OPEN_STAGES, STAGE } from "../labels";
import { Heat, HeatLegend, lastActivity } from "./heat";
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

/** What is going well on a job, when nothing needs attention. */
export function goodNews(p: Progress): string | null {
  const c = p.stageCounts;
  if (c.offer > 0) return c.offer === 1 ? "1 offer out" : `${String(c.offer)} offers out`;
  if (c.interview > 0) return `${String(c.interview)} interviewing`;
  if (c.submitted > 0) return `${String(c.submitted)} with the client`;
  return null;
}

/** The next thing to know about a job: a problem (amber or red), or good news (green). */
function Next({ why, p }: { why: string | null; p: Progress | undefined }) {
  if (why) return <span className={`next-chip ${why.includes("waiting") ? "amber" : "red"}`}>{why}</span>;
  if (!p) return null;
  const good = goodNews(p);
  if (good) return <span className="next-chip green">{good}</span>;
  return total(p) === 0 ? <span className="muted small">No candidates yet</span> : null;
}

/**
 * The jobs as a heat map, like the dashboard's: candidates per stage (a number
 * opens the job at that stage), how long each has been open, when something
 * last happened, what needs doing next, and the owner.
 */
export function JobProgressTable({ jobs, progress, empty }: {
  jobs: JobOrderSummary[]; progress?: Map<string, Progress>; empty: string;
}) {
  const [now] = useState(() => Date.now());
  const sum = (s: (typeof OPEN_STAGES)[number]) => jobs.reduce((n, j) => n + (progress?.get(j.id)?.stageCounts[s] ?? 0), 0);
  return (
    <section className="panel" aria-label="Jobs">
      <div className="table-scroll">
        <table className="heat-table job-progress">
          <thead>
            <tr>
              <th>Job</th>
              {OPEN_STAGES.map((st) => <th key={st}>{STAGE[st]}</th>)}
              <th>Open</th><th>Last activity</th><th>Next</th><th>Owner</th>
            </tr>
          </thead>
          <tbody>
            {jobs.map((j) => {
              const p = progress?.get(j.id);
              const why = attention(j, p, now);
              const last = lastActivity(p?.lastMovedAt, now);
              const open = days(j.createdAt, now);
              return (
                <tr key={j.id} className={why ? "needs-attention" : undefined}>
                  <td>
                    <a href={`#/jobs/${j.id}`} className="job-title">{j.title}</a>
                    {j.status !== "open" && <> <span className={`badge ${j.status}`}>{JOB_STATUS[j.status]}</span></>}
                    <span className="muted job-company">
                      <a href={`#/companies/${j.companyId}`} className="quiet-link">{j.companyName}</a>{j.location ? ` · ${j.location}` : ""}
                    </span>
                  </td>
                  {OPEN_STAGES.map((st) => (
                    <td key={st}>
                      {p && <Heat n={p.stageCounts[st]} waiting={p.waitingCounts[st] > 0} href={`#/jobs/${j.id}?stage=${st}`}
                        label={`${String(p.stageCounts[st])} in ${STAGE[st]} for ${j.title}`} />}
                    </td>
                  ))}
                  <td className="nowrap">{open === 1 ? "1 day" : `${String(open)} days`}</td>
                  <td className={last.old ? "last old" : "last"}>{p ? last.text : ""}</td>
                  <td className="next">{j.status === "open" && <Next why={why} p={p} />}</td>
                  <td>{j.ownerName ? <Initials name={j.ownerName} /> : <span className="muted">—</span>}</td>
                </tr>
              );
            })}
            {jobs.length > 1 && progress && (
              <tr className="total">
                <td>All shown</td>
                {OPEN_STAGES.map((st) => <td key={st}>{sum(st)}</td>)}
                <td colSpan={4} />
              </tr>
            )}
            {jobs.length === 0 && <tr><td colSpan={11} className="muted">{empty}</td></tr>}
          </tbody>
        </table>
      </div>
      <HeatLegend extra="Click a number to open the job at that stage" />
    </section>
  );
}
