import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { jobsKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { JobForm } from "../components/JobForm";
import { attention, JobProgressTable, useJobProgress } from "../components/JobProgressTable";
import { JOB_STATUS, keysOf } from "../labels";
import { hashParam } from "../hashQuery";

type StatusFilter = "" | Schemas["JobStatus"];
type View = "all" | "mine" | "attention";

/** Search, filter and add jobs. Shows open jobs first by default. */
export function JobList({ canCreate }: { canCreate: boolean }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [status, setStatus] = useState<StatusFilter>(() => (hashParam("q") ? "" : "open")); // a search looks at every job
  const [adding, setAdding] = useState(false);
  const [view, setView] = useState<View>("all");
  const mine = view === "mine";
  const list = useQuery({
    queryKey: [...jobsKey, q, status, mine],
    queryFn: async () => {
      const query: { q?: string; status?: Schemas["JobStatus"]; mine?: boolean; limit: number } = { limit: 100 };
      if (q.trim()) query.q = q.trim();
      if (status) query.status = status;
      if (mine) query.mine = true;
      return unwrap(await api.GET("/v1/jobs", { params: { query } }));
    },
  });
  const jobs = list.data?.items ?? [];
  const progress = useJobProgress(jobs.map((j) => j.id));
  const needing = jobs.filter((j) => attention(j, progress.data?.get(j.id)) !== null);
  const shown = view === "attention" ? needing : jobs;
  const views: [View, string][] = [["all", status === "open" ? "All open" : "All"], ["mine", "Mine"], ["attention", `Needs attention ${String(needing.length)}`]];

  return (
    <main>
      <div className="row spread">
        <h2>Jobs</h2>
        <div className="row">
          <input aria-label="Search jobs" placeholder="Search" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <select aria-label="Status" value={status} onChange={(e) => { setStatus(e.target.value as StatusFilter); }}>
            <option value="">All statuses</option>
            {keysOf(JOB_STATUS).map((s) => <option key={s} value={s}>{JOB_STATUS[s]}</option>)}
          </select>
          {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add job</button>}
        </div>
      </div>
      {adding && (
        <JobForm job={null} onDone={() => { setAdding(false); }} onSaved={(j) => { window.location.hash = `#/jobs/${j.id}`; }} />
      )}
      <div className="segmented job-views" role="group" aria-label="Which jobs">
        {views.map(([v, label]) => (
          <button key={v} type="button" className={view === v ? "on" : undefined} aria-pressed={view === v} onClick={() => { setView(v); }}>{label}</button>
        ))}
      </div>
      <ErrorMessage error={list.error} />
      {list.data && (
        <JobProgressTable jobs={shown} progress={progress.data}
          empty={view === "attention" ? "Nothing needs attention." : q || status || mine ? "No jobs match." : "No jobs yet."} />
      )}
    </main>
  );
}
