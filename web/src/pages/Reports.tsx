import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { formatDay, formatMoney, sourceLabel, STAGE } from "../labels";

const pct = (part: number, whole: number) => (whole > 0 ? `${String(Math.round((part * 100) / whole))}%` : "–");

/** How the business is doing over a period (the last 90 days unless chosen). */
export function Reports() {
  const [period, setPeriod] = useState({ from: "", to: "" });
  const report = useQuery({
    queryKey: ["reports", period],
    queryFn: async () => unwrap(await api.GET("/v1/reports/summary", {
      params: { query: { from: period.from || undefined, to: period.to || undefined } },
    })),
  });
  const r = report.data;
  const top = r?.funnel[0]?.reached ?? 0;
  return (
    <>
      <h2>Reports</h2>
      <div className="row">
        <label>From <input type="date" value={period.from} onChange={(e) => { setPeriod({ ...period, from: e.target.value }); }} /></label>
        <label>To <input type="date" value={period.to} onChange={(e) => { setPeriod({ ...period, to: e.target.value }); }} /></label>
        {r && <span className="muted">{formatDay(r.from)} to {formatDay(r.to)}</span>}
      </div>
      <ErrorMessage error={report.error} />
      {r && (
        <>
          <section className="panel" aria-label="Pipeline conversion">
            <h3>Pipeline conversion</h3>
            <p className="muted">Applications made in the period, by the furthest stage they reached.</p>
            <table>
              <thead><tr><th>Stage</th><th>Reached</th><th>Of the stage before</th><th aria-hidden="true" /></tr></thead>
              <tbody>
                {r.funnel.map((f) => (
                  <tr key={f.stage}>
                    <td>{STAGE[f.stage]}</td><td>{f.reached}</td>
                    <td>{f.percentOfPrevious === null ? "" : `${String(f.percentOfPrevious)}%`}</td>
                    <td aria-hidden="true"><span className="bar" style={{ width: top ? `${String((f.reached * 100) / top)}%` : 0 }} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
          <section className="panel" aria-label="Sources">
            <h3>Sources</h3>
            {r.sources.length === 0 ? <p className="muted">No applications in this period.</p> : (
              <table>
                <thead><tr><th>Source</th><th>Applications</th><th>Interviewed</th><th>Placed</th><th>Placed per application</th></tr></thead>
                <tbody>
                  {r.sources.map((s) => (
                    <tr key={s.source}>
                      <td>{sourceLabel(s.source)}</td><td>{s.applications}</td><td>{s.interviewed}</td><td>{s.placed}</td>
                      <td>{pct(s.placed, s.applications)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
          <section className="panel" aria-label="Recruiters">
            <h3>Placements per recruiter</h3>
            <p className="muted">Placements starting in the period.{r.feesHidden && " Fees are not shown to your role."}</p>
            {r.recruiters.length === 0 ? <p className="muted">No placements in this period.</p> : (
              <table>
                <thead><tr><th>Recruiter</th><th>Placements</th><th>Fell through</th>{!r.feesHidden && <th>Fees</th>}</tr></thead>
                <tbody>
                  {r.recruiters.map((x) => (
                    <tr key={x.recruiterId ?? "nobody"}>
                      <td>{x.recruiterName ?? "Nobody credited"}</td><td>{x.placements}</td><td>{x.fellThrough}</td>
                      {!r.feesHidden && <td>{x.fees.map((f) => formatMoney(f.amount, f.currency)).join(", ") || "–"}</td>}
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
          <section className="panel" aria-label="Time to fill">
            <h3>Time to fill</h3>
            <p>
              Median days from a job opening to its first placement: <strong>{r.timeToFill.medianDaysToFill ?? "–"}</strong>.{" "}
              From a candidate being added to the job to being placed: <strong>{r.timeToFill.medianDaysToPlace ?? "–"}</strong>.
            </p>
            {r.timeToFill.jobs.length > 0 && (
              <table>
                <thead><tr><th>Job</th><th>Days to fill</th><th>Placed in the period</th></tr></thead>
                <tbody>
                  {r.timeToFill.jobs.map((j) => (
                    <tr key={j.jobId}><td><a href={`#/jobs/${j.jobId}`}>{j.jobTitle}</a></td><td>{j.daysToFill}</td><td>{j.placements}</td></tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
        </>
      )}
    </>
  );
}
