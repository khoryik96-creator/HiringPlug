import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { ActivityProgress } from "../components/ActivityProgress";
import { ErrorMessage } from "../components/ErrorMessage";
import { downloadExport } from "../api/files";
import { ExportButton } from "../components/ExportButton";
import { RevenueForecast } from "../components/RevenueForecast";
import { useFieldSettings } from "../components/useFieldSettings";
import { formatDay, formatMoney } from "../labels";

type Preset = "recent" | "this_year" | "last_year" | "custom";
// Like the placements page: a few common periods, or two dates.
const PRESETS: [Preset, string][] = [["recent", "Last 90 days"], ["this_year", "This year"], ["last_year", "Last year"], ["custom", "Custom"]];

/** The dates a preset stands for ("" leaves it to the server: the last 90 days). */
export function presetDates(p: Preset, today: Date): { from: string; to: string } {
  const y = today.getFullYear();
  if (p === "this_year") return { from: `${String(y)}-01-01`, to: "" };
  if (p === "last_year") return { from: `${String(y - 1)}-01-01`, to: `${String(y - 1)}-12-31` };
  return { from: "", to: "" };
}

const pct = (part: number, whole: number) => (whole > 0 ? `${String(Math.round((part * 100) / whole))}%` : "–");

/**
 * How the business is doing over a period (the last 90 days unless chosen),
 * activity against targets, and (for those who may see fees) the revenue
 * forecast.
 */
export function Reports({ permissions = [] }: { permissions?: Permission[] }) {
  const forecast = permissions.includes("reports.forecast:read") && permissions.includes("placements.fees:read");
  const [preset, setPreset] = useState<Preset>("recent");
  const [period, setPeriod] = useState({ from: "", to: "" });
  const fs = useFieldSettings();
  const report = useQuery({
    queryKey: ["reports", period],
    queryFn: async () => unwrap(await api.GET("/v1/reports/summary", {
      params: { query: { from: period.from || undefined, to: period.to || undefined } },
    })),
  });
  const r = report.data;
  const top = r?.funnel[0]?.reached ?? 0;
  return (
    <main>
      <div className="row spread">
        <h2>Reports</h2>
        <ExportButton what="reports" run={(format) => {
          const query = new URLSearchParams({ format });
          if (period.from) query.set("from", period.from);
          if (period.to) query.set("to", period.to);
          return downloadExport(`/v1/reports/summary/export?${query.toString()}`);
        }} />
      </div>
      <ActivityProgress />
      {forecast && <RevenueForecast />}
      <div className="row spread report-period">
        <h3>Pipeline and placements</h3>
        <div className="row" role="group" aria-label="Period">
          {PRESETS.map(([key, label]) => (
            <button key={key} type="button" className="choice-pill" aria-pressed={preset === key}
              onClick={() => { setPreset(key); if (key !== "custom") setPeriod(presetDates(key, new Date())); }}>{label}</button>
          ))}
          {preset === "custom" && (
            <>
              <input type="date" aria-label="From" value={period.from} onChange={(e) => { setPeriod({ ...period, from: e.target.value }); }} />
              <input type="date" aria-label="To" value={period.to} onChange={(e) => { setPeriod({ ...period, to: e.target.value }); }} />
            </>
          )}
        </div>
      </div>
      {r && <p className="muted">{formatDay(r.from)} to {formatDay(r.to)}</p>}
      <ErrorMessage error={report.error} />
      {r && (
        <>
          <section className="panel" aria-label="Pipeline conversion">
            <h3>Pipeline conversion</h3>
            <div className="table-scroll"><table>
              <thead><tr><th>Stage</th><th>Reached</th><th>Of the stage before</th><th aria-hidden="true" className="bar-col" /></tr></thead>
              <tbody>
                {r.funnel.map((f) => (
                  <tr key={f.category}>
                    <td>{f.label}</td><td>{f.reached}</td>
                    <td>{f.percentOfPrevious === null ? "" : `${String(f.percentOfPrevious)}%`}</td>
                    <td aria-hidden="true" className="bar-col"><span className="bar" style={{ width: top ? `${String((f.reached * 100) / top)}%` : 0 }} /></td>
                  </tr>
                ))}
              </tbody>
            </table></div>
          </section>
          <section className="panel" aria-label="Sources">
            <h3>Sources</h3>
            {r.sources.length === 0 ? <p className="muted">No applications in this period.</p> : (
              <div className="table-scroll"><table>
                <thead><tr><th>Source</th><th>Applications</th><th>Interviewed</th><th>Placed</th><th>Placed per application</th></tr></thead>
                <tbody>
                  {r.sources.map((s) => (
                    <tr key={s.source}>
                      <td>{fs.label("candidate_source", s.source)}</td><td>{s.applications}</td><td>{s.interviewed}</td><td>{s.placed}</td>
                      <td>{pct(s.placed, s.applications)}</td>
                    </tr>
                  ))}
                </tbody>
              </table></div>
            )}
          </section>
          <section className="panel" aria-label="Recruiters">
            <h3>Placements per recruiter</h3>
            {r.feesHidden && <p className="muted">Fees are not shown to your role.</p>}
            {r.recruiters.length === 0 ? <p className="muted">No placements in this period.</p> : (
              <div className="table-scroll"><table>
                <thead><tr><th>Recruiter</th><th>Placements</th><th>Fell through</th>{!r.feesHidden && <th>Fees</th>}</tr></thead>
                <tbody>
                  {r.recruiters.map((x) => (
                    <tr key={x.recruiterId ?? "nobody"}>
                      <td>{x.recruiterName ?? "Nobody credited"}</td><td>{x.placements}</td><td>{x.fellThrough}</td>
                      {!r.feesHidden && <td>{x.fees.map((f) => formatMoney(f.amount, f.currency)).join(", ") || "–"}</td>}
                    </tr>
                  ))}
                </tbody>
              </table></div>
            )}
          </section>
          <section className="panel" aria-label="Time to fill">
            <h3>Time to fill</h3>
            <p>
              Median days from a job opening to its first placement: <strong>{r.timeToFill.medianDaysToFill ?? "–"}</strong>.{" "}
              From a candidate being added to the job to being placed: <strong>{r.timeToFill.medianDaysToPlace ?? "–"}</strong>.
            </p>
            {r.timeToFill.jobs.length > 0 && (
              <div className="table-scroll"><table>
                <thead><tr><th>Job</th><th>Days to fill</th><th>Placed in the period</th></tr></thead>
                <tbody>
                  {r.timeToFill.jobs.map((j) => (
                    <tr key={j.jobId}><td><a href={`#/jobs/${j.jobId}`}>{j.jobTitle}</a></td><td>{j.daysToFill}</td><td>{j.placements}</td></tr>
                  ))}
                </tbody>
              </table></div>
            )}
          </section>
        </>
      )}
    </main>
  );
}
