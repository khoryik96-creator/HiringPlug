import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { IconButton, PlusIcon } from "../components/ActionIcons";

import { api, type Schemas, unwrap } from "../api/client";
import { jobsKey } from "../api/keys";
import { downloadExport } from "../api/files";
import { ExportButton } from "../components/ExportButton";
import { ErrorMessage } from "../components/ErrorMessage";
import { takeJd } from "../components/jdHandover";
import { JobForm } from "../components/JobForm";
import { Change } from "../components/heat";
import { attention, JobProgressTable, useJobProgress } from "../components/JobProgressTable";
import { JOB_STATUS, keysOf, OPEN_STAGES } from "../labels";
import { hashParam } from "../hashQuery";

type StatusFilter = "" | Schemas["JobStatus"];
type View = "all" | "mine" | "attention";

type Progress = Schemas["JobProgress"];
const DAY = 86_400_000;

/** The numbers over the jobs shown: like the dashboard's, for these jobs only. */
function JobNumbers({ jobs, progress }: { jobs: Schemas["JobOrderSummary"][]; progress: Map<string, Progress> }) {
  const [now] = useState(() => Date.now());
  const open = jobs.filter((j) => j.status === "open");
  const ps = jobs.map((j) => progress.get(j.id)).filter((p): p is Progress => !!p);
  const add = (f: (p: Progress) => number) => ps.reduce((n, p) => n + f(p), 0);
  const inPlay = add((p) => OPEN_STAGES.filter((s) => s !== "placed").reduce((n, s) => n + p.stageCounts[s], 0));
  const waiting = add((p) => p.waiting);
  const offers = add((p) => p.stageCounts.offer);
  const placed = add((p) => p.stageCounts.placed);
  const newThisWeek = open.filter((j) => now - Date.parse(j.createdAt) < 7 * DAY).length;
  const avgOpen = open.length ? Math.round(open.reduce((n, j) => n + (now - Date.parse(j.createdAt)) / DAY, 0) / open.length) : 0;
  return (
    <div className="kpis five" role="group" aria-label="Job numbers">
      <div className="kpi"><span className="kpi-label">Open jobs</span><strong>{open.length}</strong><Change by={newThisWeek} text="{n} new this week" /></div>
      <div className="kpi"><span className="kpi-label">Candidates in play</span><strong>{inPlay}</strong><span className="kpi-note">applied to offer</span></div>
      <div className="kpi"><span className="kpi-label">Waiting over 7 days</span><strong>{waiting}</strong>
        <span className={waiting > 0 ? "kpi-note warn" : "kpi-note"}>{waiting > 0 ? "look at these first" : "nobody waiting"}</span></div>
      <div className="kpi"><span className="kpi-label">Offers out</span><strong>{offers}</strong><span className="kpi-note">{placed} placed</span></div>
      <div className="kpi"><span className="kpi-label">Average time open</span><strong>{avgOpen === 1 ? "1 day" : `${String(avgOpen)} days`}</strong>
        <span className="kpi-note">of the open jobs</span></div>
    </div>
  );
}

/** Search, filter and add jobs. Shows open jobs first by default, with their numbers on top. */
export function JobList({ canCreate, canExport = false }: { canCreate: boolean; canExport?: boolean }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [status, setStatus] = useState<StatusFilter>(() => (hashParam("q") ? "" : "open")); // a search looks at every job
  // A job description handed over from an email ("Create job"): the Add job form opens with it.
  const [handed] = useState(takeJd);
  const [adding, setAdding] = useState(handed !== null);
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
    <main className="list-page">
      <div className="row spread">
        <h2>Jobs</h2>
        <div className="row">
          <input aria-label="Search jobs" placeholder="Search jobs" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <select aria-label="Status" value={status} onChange={(e) => { setStatus(e.target.value as StatusFilter); }}>
            <option value="">All statuses</option>
            {keysOf(JOB_STATUS).map((s) => <option key={s} value={s}>{JOB_STATUS[s]}</option>)}
          </select>
          {canExport && <ExportButton what="jobs" run={(format) => {
            const query = new URLSearchParams({ format });
            if (q.trim()) query.set("q", q.trim());
            if (status) query.set("status", status);
            if (mine) query.set("mine", "true");
            if (view === "attention") for (const j of shown) query.append("ids", j.id); // only those needing attention
            return downloadExport(`/v1/jobs/export?${query.toString()}`);
          }} />}
          {canCreate && !adding && <IconButton label="Add job" icon={<PlusIcon />} className="icon-primary" onClick={() => { setAdding(true); }} />}
        </div>
      </div>
      {adding && (
        <JobForm job={null} jdFile={handed} onDone={() => { setAdding(false); }} onSaved={(j) => { window.location.hash = `#/jobs/${j.id}`; }} />
      )}
      <div className="segmented job-views" role="group" aria-label="Which jobs">
        {views.map(([v, label]) => (
          <button key={v} type="button" className={view === v ? "on" : undefined} aria-pressed={view === v} onClick={() => { setView(v); }}>{label}</button>
        ))}
      </div>
      <ErrorMessage error={list.error} />
      {list.data && progress.data && <JobNumbers jobs={shown} progress={progress.data} />}
      {list.data && (
        <JobProgressTable jobs={shown} progress={progress.data}
          empty={view === "attention" ? "Nothing needs attention." : q || status || mine ? "No jobs match." : "No jobs yet."} />
      )}
    </main>
  );
}
