import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { summaryKey } from "../api/keys";
import { ActivityProgress } from "../components/ActivityProgress";
import { CvImports } from "../components/CvImports";
import { DashboardBlocks } from "../components/DashboardBlocks";
import { Change, Heat, HeatLegend, lastActivity } from "../components/heat";
import { ErrorMessage } from "../components/ErrorMessage";
import { FirstRunGuide } from "../components/FirstRunGuide";
import { MyInterviews } from "../components/Interviews";
import { useJobProgress } from "../components/JobProgressTable";
import { MyTasks } from "../components/MyTasks";
import { NeedsYouToday } from "../components/NeedsYouToday";
import { OffersToApprove } from "../components/Offers";
import { PeoplePicker, peopleLabel, useRememberedPeople } from "../components/PeoplePicker";
import { QuickSearch } from "../components/QuickSearch";
import { OPEN_STAGES, STAGE } from "../labels";

type Counts = Schemas["StageCounts"];

const active = (c: Counts) => c.applied + c.screening + c.submitted + c.interview + c.offer;

/** The home page: search, what needs you today, importing CVs, open jobs and their pipelines. */
export function Dashboard({ permissions, meId }: { permissions: Permission[]; meId?: string }) {
  // Owners and admins choose whose numbers: theirs, some people's, or the team's.
  const canChoose = permissions.includes("reports.team:read") && permissions.includes("reports:read");
  const [chosen, choose] = useRememberedPeople(meId);
  const people = canChoose ? chosen : undefined;
  const team = useQuery({
    queryKey: ["reports", "activity", "week"],
    queryFn: async () => unwrap(await api.GET("/v1/reports/activity", { params: { query: { period: "week" } } })),
    enabled: canChoose,
  });
  const everyone = (team.data?.rows ?? []).map((r) => ({ id: r.userId, name: r.userName }));
  // The team: every open job, as before; people: the jobs they own.
  const owners = people === undefined || people === "team" ? [] : people;
  const summary = useQuery({
    queryKey: [...summaryKey, owners],
    queryFn: async () => unwrap(await api.GET("/v1/pipeline/summary", { params: { query: owners.length > 0 ? { owner: owners } : {} } })),
    placeholderData: keepPreviousData, // the last numbers stay while someone else's load
  });
  const whose = people === undefined ? "" : peopleLabel(people, new Map(everyone.map((p) => [p.id, p.name])), meId);
  const [now] = useState(() => Date.now());
  const progress = useJobProgress((summary.data?.jobs ?? []).map((j) => j.jobId));
  const s = summary.data;
  if (!s) return summary.error ? <ErrorMessage error={summary.error} /> : <p className="muted">Loading…</p>;

  return (
    <main>
      <div className="row spread">
        <h2>Dashboard</h2>
        {people !== undefined && <PeoplePicker people={people} onChange={choose} everyone={everyone} meId={meId} />}
      </div>
      <FirstRunGuide permissions={permissions} />
      <QuickSearch permissions={permissions} />
      <DashboardBlocks blocks={[
        { id: "numbers", label: "the numbers", node: (
      <div className="kpis">
        <a className="kpi" href="#/jobs"><span className="kpi-label">Open jobs</span><strong>{s.openJobs}</strong><Change by={s.openedLast7Days} text="{n} new this week" /></a>
        <div className="kpi"><span className="kpi-label">In progress</span><strong>{active(s.stageCounts)}</strong><Change by={s.addedLast7Days} text="{n} added this week" /></div>
        <div className="kpi"><span className="kpi-label">Moved this week</span><strong>{s.movedLast7Days}</strong><Change by={s.movedLast7Days - s.movedPrev7Days} text="{n} vs last week" /></div>
        <div className="kpi"><span className="kpi-label">Placed in 30 days</span><strong>{s.placedLast30Days}</strong></div>
      </div>
        ) },
        ...(permissions.includes("reports:read") && meId
          ? [{ id: "activity" as const, label: "Your activity", node: people === undefined || whose === "Me"
            ? <ActivityProgress onlyUser={meId} title="Your activity" />
            : <ActivityProgress people={people} title={`Activity: ${whose}`} /> }] : []),
        { id: "needsYou", label: "Needs you today", node: <NeedsYouToday permissions={permissions} jobs={s.jobs} progress={progress.data} /> },
        ...(permissions.includes("intake:create") ? [{ id: "import" as const, label: "Import profiles", node: <CvImports foldedHistory /> }] : []),
        ...(permissions.includes("automations.tasks:read")
          ? [{ id: "tasks" as const, label: "Your tasks", node: <MyTasks approvals={permissions.includes("automations:approve")} /> }] : []),
        ...(permissions.includes("offers:approve")
          ? [{ id: "offersToApprove" as const, label: "Offers to approve", node: <OffersToApprove permissions={permissions} /> }] : []),
        ...(permissions.includes("interviews:read")
          ? [{ id: "interviews" as const, label: "My upcoming interviews", node: <MyInterviews permissions={permissions} /> }] : []),
        { id: "openJobs", label: "Open jobs", node: (
      <section className="panel" aria-label="Open jobs">
        <h3>{whose === "" || whose === "Whole team" ? "Open jobs" : `Open jobs: ${whose}`}</h3>
        <div className="table-scroll">
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
                  <td><a href={`#/jobs/${j.jobId}`}><strong>{j.title}</strong></a><span className="muted job-company">{j.companyName}</span></td>
                  {OPEN_STAGES.map((st) => (
                    <td key={st}>
                      <Heat n={j.stageCounts[st]} waiting={(p?.waitingCounts[st] ?? 0) > 0} href={`#/jobs/${j.jobId}?stage=${st}`}
                        label={`${String(j.stageCounts[st])} in ${STAGE[st]} for ${j.title}`} />
                    </td>
                  ))}
                  <td className={last.old ? "last old" : "last"}>{p ? last.text : ""}</td>
                </tr>
              );
            })}
            <tr className="total">
              <td>All open jobs</td>
              {OPEN_STAGES.map((st) => <td key={st}>{s.stageCounts[st]}</td>)}
              <td />
            </tr>
            {s.jobs.length === 0 && <tr><td colSpan={8} className="muted">{owners.length > 0 ? `No open jobs owned by ${whose}.` : "No open jobs. Add one under Jobs."}</td></tr>}
          </tbody>
        </table>
        </div>
        <HeatLegend />
      </section>
        ) },
      ]} />
    </main>
  );
}
