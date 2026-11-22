// Activity against targets: calls logged, CVs sent to clients, interviews
// arranged and placements, this week or this month. A recruiter sees their
// own; owners and admins see everyone's (the Reports page) or their own (the
// dashboard).
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { formatDay } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Period = Schemas["ActivityPeriod"];
type Counts = Schemas["ActivityCounts"];
type Goal = Schemas["ActivityGoal"];

export const ACTIVITIES: [keyof Counts, string][] = [
  ["calls", "Calls"], ["cvsSent", "CVs sent"], ["interviews", "Interviews arranged"], ["placements", "Placements"],
];

export const activityKey = (period: Period) => ["reports", "activity", period];

/** "3 of 20" and how far along (0 to 100), or just "3" without a target. */
export function progress(done: number, target: number | null | undefined): { text: string; percent: number | null } {
  if (target === null || target === undefined) return { text: String(done), percent: null };
  if (target === 0) return { text: `${String(done)} of 0`, percent: 100 };
  return { text: `${String(done)} of ${String(target)}`, percent: Math.min(100, Math.round((done * 100) / target)) };
}

function Meter({ label, done, target }: { label: string; done: number; target: number | null | undefined }) {
  const p = progress(done, target);
  return (
    <span className="meter-cell">
      <span>{p.text}</span>
      {p.percent !== null && (
        <span className={p.percent >= 100 ? "meter met" : "meter"} role="meter" aria-label={label}
          aria-valuemin={0} aria-valuemax={100} aria-valuenow={p.percent}>
          <span style={{ width: `${String(p.percent)}%` }} />
        </span>
      )}
    </span>
  );
}

const goalOf = (row: Schemas["ActivityRow"]): Goal | null => row.target;

/** A panel of activity against targets; onlyUser shows just that person (the dashboard). */
export function ActivityProgress({ onlyUser, title = "Activity against targets" }: { onlyUser?: string; title?: string }) {
  const [period, setPeriod] = useState<Period>("week");
  const activity = useQuery({
    queryKey: activityKey(period),
    queryFn: async () => unwrap(await api.GET("/v1/reports/activity", { params: { query: { period } } })),
  });
  const a = activity.data;
  const rows = (a?.rows ?? []).filter((r) => !onlyUser || r.userId === onlyUser);
  const single = rows.length === 1 && (onlyUser !== undefined || !a?.team);
  const me = single ? rows[0] : undefined;
  return (
    <section className="panel" aria-label={title}>
      <div className="row spread">
        <h3>{title}</h3>
        <div className="row" role="group" aria-label="Activity period">
          {(["week", "month"] as const).map((p) => (
            <button key={p} type="button" className="choice-pill" aria-pressed={period === p} onClick={() => { setPeriod(p); }}>
              {p === "week" ? "This week" : "This month"}
            </button>
          ))}
        </div>
      </div>
      <ErrorMessage error={activity.error} />
      {a && <p className="muted small">{formatDay(a.from)} to {formatDay(a.to)}</p>}
      {me && (
        <div className="activity-cards">
          {ACTIVITIES.map(([key, label]) => {
            const target = goalOf(me)?.[key];
            return (
              <div className="activity-card" key={key}>
                <span className="kpi-label">{label}</span>
                <strong>{me.done[key]}</strong>
                <Meter label={`${label}: progress`} done={me.done[key]} target={target} />
                <small className="muted">{target === null || target === undefined ? "No target set" : `Target ${String(target)}`}</small>
              </div>
            );
          })}
        </div>
      )}
      {!me && rows.length > 0 && (
        <div className="table-scroll"><table>
          <thead><tr><th>Who</th>{ACTIVITIES.map(([key, label]) => <th key={key}>{label}</th>)}</tr></thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.userId}>
                <td>{r.userName}</td>
                {ACTIVITIES.map(([key, label]) => (
                  <td key={key}><Meter label={`${r.userName}, ${label}: progress`} done={r.done[key]} target={goalOf(r)?.[key]} /></td>
                ))}
              </tr>
            ))}
          </tbody>
        </table></div>
      )}
      {a && rows.length === 0 && <p className="muted">Nobody to show.</p>}
      {a && (
        <p className="muted small">
          Calls: screening calls, reference checks and call notes. CVs sent: candidates moved to Submitted, and CVs emailed (a candidate once a day).
          {a.rows.every((r) => r.target === null) && " No targets are set yet: owners and admins set them in Settings, Targets & forecast."}
        </p>
      )}
    </section>
  );
}
