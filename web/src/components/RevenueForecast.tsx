// The revenue forecast: fees expected from the open pipeline and from
// placements not yet invoiced, each weighted by its stage, per month and per
// recruiter. Owners and admins only (it shows fees).
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { formatMoney, STAGE } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Total = Schemas["ForecastTotal"];

export const forecastKey = ["reports", "forecast"];

/** "November 2026" for "2026-11". */
export function monthName(m: string): string {
  const [y, mo] = m.split("-").map(Number);
  return new Date(Date.UTC(y ?? 2000, (mo ?? 1) - 1, 1)).toLocaleDateString("en-GB", { month: "long", year: "numeric", timeZone: "UTC" });
}

const money = (ts: Total[], pick: "expected" | "full") => ts.map((t) => formatMoney(t[pick], t.currency)).join(", ") || "–";

const GAP: Record<Schemas["ForecastGap"]["reason"], string> = {
  no_salary_budget: "the job has no monthly or yearly salary budget",
  no_terms: "the client has no terms of business with a fee",
  no_rate: "none of the client's rates fits the job's salary",
};

export function RevenueForecast() {
  const forecast = useQuery({ queryKey: forecastKey, queryFn: async () => unwrap(await api.GET("/v1/reports/forecast")) });
  const f = forecast.data;
  const weights = f?.stageWeights.map((w) => `${STAGE[w.stage]} ${String(w.percent)}%`).join(", ");
  return (
    <section className="panel" aria-label="Revenue forecast">
      <h3>Revenue forecast</h3>
      <ErrorMessage error={forecast.error} />
      {f && (
        <>
          <p className="muted small">
            Fees expected from today's pipeline, each weighted by how far the candidate has got ({weights}). A placement counts its recorded fee until
            it is invoiced; others count the client's rate on the job's salary budget. Weights are set in Settings, Targets &amp; forecast.
          </p>
          {f.lines.length === 0 ? <p className="muted">Nothing expected yet: no candidate is submitted, interviewing, offered or placed without an invoice.</p> : (
            <div className="forecast-grid">
              <div className="table-scroll"><table aria-label="Forecast by month">
                <thead><tr><th>Month</th><th>Expected</th><th>Full fees</th></tr></thead>
                <tbody>
                  {f.months.map((m) => (
                    <tr key={m.month}><td>{monthName(m.month)}</td><td><strong>{money(m.totals, "expected")}</strong></td><td className="muted">{money(m.totals, "full")}</td></tr>
                  ))}
                </tbody>
              </table></div>
              <div className="table-scroll"><table aria-label="Forecast by recruiter">
                <thead><tr><th>Recruiter</th><th>Expected</th><th>Full fees</th></tr></thead>
                <tbody>
                  {f.recruiters.map((r) => (
                    <tr key={r.recruiterId ?? "none"}>
                      <td>{r.recruiterName ?? "No recruiter"}</td><td><strong>{money(r.totals, "expected")}</strong></td><td className="muted">{money(r.totals, "full")}</td>
                    </tr>
                  ))}
                </tbody>
              </table></div>
            </div>
          )}
          {f.lines.length > 0 && (
            <details>
              <summary>Each candidate counted ({f.lines.length})</summary>
              <div className="table-scroll"><table>
                <thead><tr><th>Candidate</th><th>Job</th><th>Stage</th><th>Recruiter</th><th>Month</th><th>Fee</th><th>Weight</th><th>Expected</th></tr></thead>
                <tbody>
                  {f.lines.map((l) => (
                    <tr key={l.applicationId}>
                      <td>{l.candidateName}</td><td>{l.jobTitle}<br /><span className="muted small">{l.companyName}</span></td><td>{STAGE[l.stage]}</td>
                      <td>{l.recruiterName ?? "–"}</td><td className="nowrap">{monthName(l.month)}</td>
                      <td className="nowrap" title={l.feeFrom === "placement" ? "The fee recorded on the placement" : "The client's rate on the job's salary budget"}>
                        {formatMoney(l.fee, l.currency)}{l.feeFrom === "placement" && <><br /><span className="muted small">recorded on the placement</span></>}
                      </td>
                      <td>{l.percent}%</td><td className="nowrap"><strong>{formatMoney(l.expected, l.currency)}</strong></td>
                    </tr>
                  ))}
                </tbody>
              </table></div>
            </details>
          )}
          {f.gaps.length > 0 && (
            <details>
              <summary>Not counted: no fee could be worked out ({f.gaps.length})</summary>
              <ul>
                {f.gaps.map((g) => (
                  <li key={g.applicationId}>{g.candidateName}, {g.jobTitle} ({STAGE[g.stage]}): {GAP[g.reason]}</li>
                ))}
              </ul>
            </details>
          )}
        </>
      )}
    </section>
  );
}
