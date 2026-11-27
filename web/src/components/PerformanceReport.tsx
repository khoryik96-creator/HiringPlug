// Performance over time on the Reports page: billings, CVs sent, each stage
// category and placements by week, month, quarter or year, with the totals
// (year to date by default), conversions from one category to the next, and
// the average days from a job opening to its first CV and to its first
// placement. Owners and admins choose whose numbers.
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { formatDay, formatMoney } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { type People, PeoplePicker, peopleLabel } from "./PeoplePicker";
import { targetCategories, useStageCategories } from "./stages/categories";

type Grain = Schemas["PerformanceGrain"];
type Row = Schemas["PerformanceRow"];
type Range = "ytd" | "last_year" | "last_12" | "custom";

const GRAINS: [Grain, string][] = [["week", "Week"], ["month", "Month"], ["quarter", "Quarter"], ["year", "Year"]];
const RANGES: [Range, string][] = [["ytd", "This year to date"], ["last_year", "Last year"], ["last_12", "Last 12 months"], ["custom", "Custom"]];

const iso = (d: Date) => d.toISOString().slice(0, 10);

/** The days a range stands for ("" leaves it to the server: this year so far). */
export function rangeDates(r: Range, today: Date): { from: string; to: string } {
  const y = today.getUTCFullYear();
  if (r === "last_year") return { from: `${String(y - 1)}-01-01`, to: `${String(y - 1)}-12-31` };
  if (r === "last_12") return { from: iso(new Date(Date.UTC(y, today.getUTCMonth() - 11, 1))), to: "" };
  return { from: "", to: "" };
}

/** What a period is called: "Nov 2026", "Q4 2026", "2026", or the week's first day. */
export function periodLabel(grain: Grain, from: string): string {
  const d = new Date(`${from}T00:00:00Z`);
  const y = String(d.getUTCFullYear());
  if (grain === "year") return y;
  if (grain === "quarter") return `Q${String(Math.floor(d.getUTCMonth() / 3) + 1)} ${y}`;
  if (grain === "month") return d.toLocaleDateString("en-GB", { month: "short", year: "numeric", timeZone: "UTC" });
  return `Week of ${formatDay(from)}`;
}

const money = (fees: Row["fees"]) => fees.map((f) => formatMoney(f.amount, f.currency)).join(" · ") || "–";
const jobs = (n: number) => `${String(n)} ${n === 1 ? "job" : "jobs"}`;
const days = (d: Schemas["PerformanceDays"]) => (d.average === null ? "–" : `${String(d.average)} days`);

export function PerformanceReport({ canChoose, meId }: { canChoose: boolean; meId?: string }) {
  const [grain, setGrain] = useState<Grain>("month");
  const [range, setRange] = useState<Range>("ytd");
  const [dates, setDates] = useState({ from: "", to: "" });
  const [people, setPeople] = useState<People>("team");
  const allCats = useStageCategories().data?.items ?? [];
  const cats = targetCategories(allCats);
  const team = useQuery({
    queryKey: ["reports", "activity", "week"],
    queryFn: async () => unwrap(await api.GET("/v1/reports/activity", { params: { query: { period: "week" } } })),
    enabled: canChoose,
  });
  const everyone = (team.data?.rows ?? []).map((r) => ({ id: r.userId, name: r.userName }));
  const users = canChoose && people !== "team" ? people : undefined;
  const perf = useQuery({
    queryKey: ["reports", "performance", grain, dates, users],
    queryFn: async () => unwrap(await api.GET("/v1/reports/performance", {
      params: { query: { grain, from: dates.from || undefined, to: dates.to || undefined, user: users } },
    })),
    placeholderData: keepPreviousData,
  });
  const p = perf.data;
  const label = (key: string) => allCats.find((c) => c.key === key)?.label ?? key;
  const whose = canChoose ? peopleLabel(people, new Map(everyone.map((x) => [x.id, x.name])), meId) : "Me";
  const totalName = range === "ytd" ? "Year to date" : "Total";
  return (
    <section className="panel" aria-label="Performance">
      <div className="row spread">
        <h3>Performance{whose === "Whole team" ? "" : `: ${whose}`}</h3>
        {canChoose && <PeoplePicker people={people} onChange={setPeople} everyone={everyone} meId={meId} />}
      </div>
      <div className="row report-controls">
        <div className="row" role="group" aria-label="By">
          {GRAINS.map(([g, text]) => (
            <button key={g} type="button" className="choice-pill" aria-pressed={grain === g} onClick={() => { setGrain(g); }}>{text}</button>
          ))}
        </div>
        <div className="row" role="group" aria-label="Range">
          {RANGES.map(([r, text]) => (
            <button key={r} type="button" className="choice-pill" aria-pressed={range === r}
              onClick={() => { setRange(r); if (r !== "custom") setDates(rangeDates(r, new Date())); }}>{text}</button>
          ))}
          {range === "custom" && (
            <>
              <input type="date" aria-label="Performance from" value={dates.from} onChange={(e) => { setDates({ ...dates, from: e.target.value }); }} />
              <input type="date" aria-label="Performance to" value={dates.to} onChange={(e) => { setDates({ ...dates, to: e.target.value }); }} />
            </>
          )}
        </div>
      </div>
      <ErrorMessage error={perf.error} />
      {p && (
        <>
          <p className="muted small">{formatDay(p.from)} to {formatDay(p.to)}</p>
          <div className="kpis five">
            {!p.feesHidden && <div className="kpi"><span className="kpi-label">Billings ({totalName.toLowerCase()})</span><strong className="kpi-money">{money(p.total.fees)}</strong></div>}
            <div className="kpi"><span className="kpi-label">Placements</span><strong>{p.total.placements}</strong></div>
            <div className="kpi"><span className="kpi-label">CVs sent</span><strong>{p.total.cvsSent}</strong></div>
            <div className="kpi"><span className="kpi-label">Job open to first CV</span><strong>{days(p.daysToFirstCv)}</strong>
              <small className="muted">average, {jobs(p.daysToFirstCv.jobs)}</small></div>
            <div className="kpi"><span className="kpi-label">Job open to placement</span><strong>{days(p.daysToFill)}</strong>
              <small className="muted">average, {jobs(p.daysToFill.jobs)}</small></div>
          </div>
          <div className="table-scroll"><table className="performance-table">
            <thead><tr>
              <th>{GRAINS.find(([g]) => g === p.grain)?.[1]}</th>{!p.feesHidden && <th>Billings</th>}<th>Placements</th><th>CVs sent</th>
              {cats.map((c) => <th key={c.key}>{c.label}</th>)}
            </tr></thead>
            <tbody>
              {[...p.rows, p.total].map((r, i) => {
                const total = i === p.rows.length;
                return (
                  <tr key={total ? "total" : r.from} className={total ? "total" : undefined}>
                    <td>{total ? totalName : periodLabel(p.grain, r.from)}</td>
                    {!p.feesHidden && <td className="nowrap">{money(r.fees)}</td>}<td>{r.placements}</td><td>{r.cvsSent}</td>
                    {cats.map((c) => <td key={c.key}>{r.categories[c.key] ?? 0}</td>)}
                  </tr>
                );
              })}
            </tbody>
          </table></div>
          {p.feesHidden && <p className="muted small">Billings are not shown to your role.</p>}
          <h4>Conversion</h4>
          <p className="muted small">Of the candidates moved into a stage in this range, how many have reached the next one (or gone further) so far.</p>
          <div className="conversions">
            {p.conversions.map((c) => (
              <div className="conversion" key={`${c.from}>${c.to}`}>
                <span>{label(c.from)} → {label(c.to)}</span>
                <strong>{c.percent === null ? "–" : `${String(c.percent)}%`}</strong>
                <small className="muted">{c.reached} of {c.entered}</small>
              </div>
            ))}
          </div>
          <p className="muted small">
            CVs sent: candidates moved to Submitted and CVs emailed, once per candidate a day. A stage counts the candidates moved into it, once per
            candidate in a period. Placements and billings go by the start date. The times are over the jobs owned by those shown.
          </p>
        </>
      )}
    </section>
  );
}
