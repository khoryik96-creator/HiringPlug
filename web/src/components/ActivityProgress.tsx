// Activity against targets: calls logged, CVs sent to clients, interviews
// arranged and placements, this week or this month. A recruiter sees their
// own; owners and admins see everyone's (the Reports page) or their own (the
// dashboard).
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { formatDay } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import type { People } from "./PeoplePicker";
import { targetCategories, useStageCategories } from "./stages/categories";

type Period = Schemas["ActivityPeriod"];
type Counts = Schemas["ActivityCounts"];
type Goal = Schemas["ActivityGoal"];

type Activity = Exclude<keyof Counts, "categories">;

export const ACTIVITIES: [Activity, string][] = [
  ["calls", "Calls"], ["cvsSent", "CVs sent"], ["interviews", "Interviews arranged"], ["placements", "Placements"],
];

export const activityKey = (period: Period) => ["reports", "activity", period];

/** "3 of 20" and how far along (0 to 100), or just "3" without a target. */
export function progress(done: number, target: number | null | undefined): { text: string; percent: number | null } {
  if (target === null || target === undefined) return { text: String(done), percent: null };
  if (target === 0) return { text: `${String(done)} of 0`, percent: 100 };
  return { text: `${String(done)} of ${String(target)}`, percent: Math.min(100, Math.round((done * 100) / target)) };
}

function Meter({ label, done, target, bare = false }: { label: string; done: number; target: number | null | undefined; bare?: boolean }) {
  const p = progress(done, target);
  return (
    <span className="meter-cell">
      {!bare && <span>{p.text}</span>}
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

/** One thing counted against a target: an activity, or moves into a stage category. */
interface Measure { key: string; label: string; done: (r: Schemas["ActivityRow"]) => number; target: (r: Schemas["ActivityRow"]) => number | null | undefined }

/** The activities, then each stage category someone shown has a target for. */
export function measuresOf(rows: Schemas["ActivityRow"][], cats: Schemas["StageCategory"][]): Measure[] {
  const out: Measure[] = ACTIVITIES.map(([k, label]) => ({ key: k, label, done: (r) => r.done[k], target: (r) => goalOf(r)?.[k] }));
  for (const c of targetCategories(cats)) {
    if (!rows.some((r) => goalOf(r)?.categories?.[c.key] !== undefined)) continue;
    out.push({ key: "category:" + c.key, label: c.label, done: (r) => r.done.categories[c.key] ?? 0, target: (r) => goalOf(r)?.categories?.[c.key] });
  }
  return out;
}

/**
 * Several people's activity added up, with their targets added up (null
 * where none of them has one).
 */
export function sumRows(rows: Schemas["ActivityRow"][], userName: string): Schemas["ActivityRow"] {
  const done: Counts = { calls: 0, cvsSent: 0, interviews: 0, placements: 0, categories: {} };
  const target: Goal = { calls: null, cvsSent: null, interviews: null, placements: null };
  const add = (a: number | null | undefined, b: number | null | undefined) => (b === null || b === undefined ? a ?? null : (a ?? 0) + b);
  for (const r of rows) {
    for (const [k] of ACTIVITIES) {
      done[k] += r.done[k];
      target[k] = add(target[k], r.target?.[k]);
    }
    for (const [key, n] of Object.entries(r.done.categories)) done.categories[key] = (done.categories[key] ?? 0) + n;
    for (const [key, n] of Object.entries(r.target?.categories ?? {})) target.categories = { ...target.categories, [key]: (target.categories?.[key] ?? 0) + n };
  }
  const any = rows.some((r) => r.target !== null);
  return { userId: "total", userName, done, target: any ? target : null };
}

/**
 * A panel of activity against targets. people chooses whose: one person
 * (cards), several (a row each and their total), or the whole team (its
 * total); onlyUser is one person. Without either, everyone listed.
 */
export function ActivityProgress({ onlyUser, people, title = "Activity against targets" }: { onlyUser?: string; people?: People; title?: string }) {
  const chosen: People | undefined = people ?? (onlyUser ? [onlyUser] : undefined);
  const [period, setPeriod] = useState<Period>("week");
  const activity = useQuery({
    queryKey: activityKey(period),
    queryFn: async () => unwrap(await api.GET("/v1/reports/activity", { params: { query: { period } } })),
  });
  const a = activity.data;
  const all = a?.rows ?? [];
  const rows = chosen === undefined || chosen === "team" ? all : all.filter((r) => chosen.includes(r.userId));
  const single = rows.length === 1 && (chosen !== undefined || !a?.team);
  // The team's total once there is something to add up (not zeros while loading or on an error).
  const me = chosen === "team" ? (a ? sumRows(all, "Whole team") : undefined) : single ? rows[0] : undefined;
  const total = Array.isArray(chosen) && rows.length > 1 ? sumRows(rows, "Together") : undefined;
  const measures = measuresOf(rows, useStageCategories().data?.items ?? []);
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
          {measures.map(({ key, label, done, target: targetOf }) => {
            const target = targetOf(me);
            return (
              <div className="activity-card" key={key}>
                <span className="kpi-label">{label}</span>
                <strong>{done(me)}</strong>
                <Meter label={`${label}: progress`} done={done(me)} target={target} bare />
                <small className="muted">{target === null || target === undefined ? "No target set" : `of ${String(target)}`}</small>
              </div>
            );
          })}
        </div>
      )}
      {!me && rows.length > 0 && (
        <div className="table-scroll"><table>
          <thead><tr><th>Who</th>{measures.map(({ key, label }) => <th key={key}>{label}</th>)}</tr></thead>
          <tbody>
            {[...rows, ...(total ? [total] : [])].map((r) => (
              <tr key={r.userId} className={r === total ? "total" : undefined}>
                <td>{r.userName}</td>
                {measures.map(({ key, label, done, target }) => (
                  <td key={key}><Meter label={`${r.userName}, ${label}: progress`} done={done(r)} target={target(r)} /></td>
                ))}
              </tr>
            ))}
          </tbody>
        </table></div>
      )}
      {a && !me && rows.length === 0 && <p className="muted">Nobody to show.</p>}
      {a && (
        <p className="muted small">
          Calls: screening calls, reference checks and call notes. CVs sent: candidates moved to Submitted, and CVs emailed (a candidate once a day).
          {a.rows.every((r) => r.target === null) && " No targets are set yet: owners and admins set them in Settings, Targets & forecast."}
        </p>
      )}
    </section>
  );
}
