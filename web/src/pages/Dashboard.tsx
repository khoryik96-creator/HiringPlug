import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { summaryKey } from "../api/keys";
import { CvImports } from "../components/CvImports";
import { ErrorMessage } from "../components/ErrorMessage";
import { MyInterviews } from "../components/Interviews";
import { useJobProgress } from "../components/JobProgressTable";
import { MyTasks } from "../components/MyTasks";
import { NeedsYouToday } from "../components/NeedsYouToday";
import { OffersToApprove } from "../components/Offers";
import { QuickSearch } from "../components/QuickSearch";
import { OPEN_STAGES, STAGE } from "../labels";

type Counts = Schemas["StageCounts"];

const active = (c: Counts) => c.applied + c.screening + c.submitted + c.interview + c.offer;

/** "▲ 3 this week", "▼ 5 vs last week": a change, coloured by direction. */
function Change({ by, text }: { by: number; text: string }) {
  if (by === 0) return <span className="change flat">{text.replace("{n}", "no change")}</span>;
  return <span className={`change ${by > 0 ? "up" : "down"}`}>{by > 0 ? "▲" : "▼"} {text.replace("{n}", String(Math.abs(by)))}</span>;
}

/** Heat: stronger purple for more people; an amber ring when someone there waits over 7 days. */
function Heat({ n, waiting }: { n: number; waiting: boolean }) {
  if (n === 0) return <span className="heat h0" aria-label="none">·</span>;
  const level = n >= 4 ? "h3" : n >= 2 ? "h2" : "h1";
  return <span className={`heat ${level}${waiting ? " stall" : ""}`} title={waiting ? "Someone here has waited over 7 days" : undefined}>{n}</span>;
}

function lastActivity(iso: string | null | undefined, now: number): { text: string; old: boolean } {
  if (!iso) return { text: "no candidates", old: true };
  const days = Math.floor((now - Date.parse(iso)) / 86_400_000);
  return { text: days < 1 ? "today" : days === 1 ? "yesterday" : `${String(days)} days ago`, old: days >= 7 };
}

/** The home page: search, what needs you today, importing CVs, open jobs and their pipelines. */
export function Dashboard({ permissions }: { permissions: Permission[] }) {
  const summary = useQuery({
    queryKey: summaryKey,
    queryFn: async () => unwrap(await api.GET("/v1/pipeline/summary")),
  });
  const [now] = useState(() => Date.now());
  const progress = useJobProgress((summary.data?.jobs ?? []).map((j) => j.jobId));
  const s = summary.data;
  if (!s) return summary.error ? <ErrorMessage error={summary.error} /> : <p className="muted">Loading…</p>;

  return (
    <main>
      <h2>Dashboard</h2>
      <QuickSearch permissions={permissions} />
      <div className="kpis">
        <div className="kpi"><span className="kpi-label">Open jobs</span><strong>{s.openJobs}</strong><Change by={s.openedLast7Days} text="{n} new this week" /></div>
        <div className="kpi"><span className="kpi-label">In progress</span><strong>{active(s.stageCounts)}</strong><Change by={s.addedLast7Days} text="{n} added this week" /></div>
        <div className="kpi"><span className="kpi-label">Moved this week</span><strong>{s.movedLast7Days}</strong><Change by={s.movedLast7Days - s.movedPrev7Days} text="{n} vs last week" /></div>
        <div className="kpi"><span className="kpi-label">Placed in 30 days</span><strong>{s.placedLast30Days}</strong></div>
      </div>

      <NeedsYouToday permissions={permissions} jobs={s.jobs} progress={progress.data} />
      {permissions.includes("intake:create") && <CvImports foldedHistory />}
      {permissions.includes("automations.tasks:read") && <MyTasks approvals={permissions.includes("automations:approve")} />}
      {permissions.includes("offers:approve") && <OffersToApprove permissions={permissions} />}
      {permissions.includes("interviews:read") && <MyInterviews permissions={permissions} />}

      <section className="panel" aria-label="Open jobs">
        <h3>Open jobs</h3>
        <table className="heat-table">
          <thead>
            <tr>
              <th>Job</th>
              {OPEN_STAGES.map((st) => <th key={st}>{STAGE[st]}</th>)}
              <th className="last">Last activity</th>
            </tr>
          </thead>
          <tbody>
            {s.jobs.map((j) => {
              const p = progress.data?.get(j.jobId);
              const last = lastActivity(p?.lastMovedAt, now);
              return (
                <tr key={j.jobId}>
                  <td><a href={`#/jobs/${j.jobId}`}><strong>{j.title}</strong></a> <span className="muted">{j.companyName}</span></td>
                  {OPEN_STAGES.map((st) => <td key={st}><Heat n={j.stageCounts[st]} waiting={(p?.waitingCounts[st] ?? 0) > 0} /></td>)}
                  <td className={last.old ? "last old" : "last"}>{p ? last.text : ""}</td>
                </tr>
              );
            })}
            <tr className="total">
              <td>All open jobs</td>
              {OPEN_STAGES.map((st) => <td key={st}>{s.stageCounts[st]}</td>)}
              <td />
            </tr>
            {s.jobs.length === 0 && <tr><td colSpan={8} className="muted">No open jobs. Add one under Jobs.</td></tr>}
          </tbody>
        </table>
        <p className="heat-legend muted small">
          <span><i className="heat-key h1" />1</span><span><i className="heat-key h2" />2–3</span><span><i className="heat-key h3" />4+</span>
          <span><i className="heat-key ring" />someone waiting over 7 days</span>
        </p>
      </section>
    </main>
  );
}
