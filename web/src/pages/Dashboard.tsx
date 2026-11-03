import { useQuery } from "@tanstack/react-query";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { summaryKey } from "../api/keys";
import { timeInStage } from "../components/ApplicationCard";
import { ErrorMessage } from "../components/ErrorMessage";
import { MyInterviews } from "../components/Interviews";
import { OffersToApprove } from "../components/Offers";
import { OPEN_STAGES, STAGE } from "../labels";

type Counts = Schemas["StageCounts"];

const active = (c: Counts) => c.applied + c.screening + c.submitted + c.interview + c.offer;

/** The home page: open jobs and their pipelines, who is stuck, recent results. */
export function Dashboard({ permissions }: { permissions: Permission[] }) {
  const summary = useQuery({
    queryKey: summaryKey,
    queryFn: async () => unwrap(await api.GET("/v1/pipeline/summary")),
  });
  const s = summary.data;
  if (!s) return summary.error ? <ErrorMessage error={summary.error} /> : <p className="muted">Loading…</p>;

  return (
    <main>
      <h2>Dashboard</h2>
      <div className="stats">
        <div className="stat"><strong>{s.openJobs}</strong><span>open jobs</span></div>
        <div className="stat"><strong>{active(s.stageCounts)}</strong><span>candidates in progress</span></div>
        <div className="stat"><strong>{s.movedLast7Days}</strong><span>moved this week</span></div>
        <div className="stat"><strong>{s.placedLast30Days}</strong><span>placed in 30 days</span></div>
      </div>

      {permissions.includes("offers:approve") && <OffersToApprove permissions={permissions} />}
      {permissions.includes("interviews:read") && <MyInterviews permissions={permissions} />}

      <section className="panel" aria-label="Open jobs">
        <h3>Open jobs</h3>
        <table>
          <thead>
            <tr>
              <th>Job</th>
              {OPEN_STAGES.map((st) => <th key={st}>{STAGE[st]}</th>)}
            </tr>
          </thead>
          <tbody>
            {s.jobs.map((j) => (
              <tr key={j.jobId}>
                <td><a href={`#/jobs/${j.jobId}`}>{j.title}</a> <span className="muted">{j.companyName}</span></td>
                {OPEN_STAGES.map((st) => <td key={st}>{j.stageCounts[st] || ""}</td>)}
              </tr>
            ))}
            <tr className="total">
              <td>All open jobs</td>
              {OPEN_STAGES.map((st) => <td key={st}>{s.stageCounts[st]}</td>)}
            </tr>
            {s.jobs.length === 0 && <tr><td colSpan={7} className="muted">No open jobs. Add one under Jobs.</td></tr>}
          </tbody>
        </table>
      </section>

      <section className="panel" aria-label="Waiting too long">
        <h3>Waiting too long</h3>
        {s.stuck.length === 0 ? (
          <p className="muted">Nobody has waited more than 14 days in one stage.</p>
        ) : (
          <ul>
            {s.stuck.map((a) => (
              <li key={a.id}>
                <a href={`#/candidates/${a.candidateId}`}>{a.candidateName}</a>{" "}
                <span className={`badge ${a.stage}`}>{STAGE[a.stage]}</span>{" "}
                for {timeInStage(a)} on <a href={`#/jobs/${a.jobId}`}>{a.jobTitle}</a> <span className="muted">({a.companyName})</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
