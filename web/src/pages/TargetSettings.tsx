// Settings, Targets & forecast: each recruiter's activity targets per week
// and per month (or one target for everyone), and how much the revenue
// forecast counts a candidate at each stage.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ACTIVITIES } from "../components/ActivityProgress";
import { ErrorMessage } from "../components/ErrorMessage";
import { fieldErrors } from "../components/forms";
import { STAGE } from "../labels";

type Period = Schemas["ActivityPeriod"];
type Goal = Schemas["ActivityGoal"];
type Draft = Record<keyof Goal, string>;
type Weight = Schemas["StageWeight"];

export const reportSettingsKey = ["reports", "settings"];
const EVERYONE = "";
const EMPTY: Draft = { calls: "", cvsSent: "", interviews: "", placements: "" };

const keyOf = (userId: string | null, period: Period) => `${userId ?? EVERYONE}|${period}`;

/** The drafts of saved targets, by person and period. */
export function draftsOf(targets: Schemas["ActivityTarget"][]): Record<string, Draft> {
  const out: Record<string, Draft> = {};
  for (const t of targets) {
    const d = { ...EMPTY };
    for (const [k] of ACTIVITIES) d[k] = t.goal[k] === null ? "" : String(t.goal[k]);
    out[keyOf(t.userId, t.period)] = d;
  }
  return out;
}

/** The targets to save: every person and period with at least one number. */
export function targetsOf(drafts: Record<string, Draft>): Schemas["ActivityTarget"][] {
  const out: Schemas["ActivityTarget"][] = [];
  for (const [key, d] of Object.entries(drafts)) {
    if (ACTIVITIES.every(([k]) => d[k].trim() === "")) continue;
    const [user = "", period = "week"] = key.split("|");
    const goal = { calls: null, cvsSent: null, interviews: null, placements: null } as Goal;
    for (const [k] of ACTIVITIES) goal[k] = d[k].trim() === "" ? null : Number(d[k]);
    out.push({ userId: user === EVERYONE ? null : user, period: period as Period, goal });
  }
  return out;
}

export function TargetSettings() {
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: reportSettingsKey, queryFn: async () => unwrap(await api.GET("/v1/reports/settings")) });
  // The company's users, as the activity report lists them.
  const team = useQuery({
    queryKey: ["reports", "activity", "week"],
    queryFn: async () => unwrap(await api.GET("/v1/reports/activity", { params: { query: { period: "week" } } })),
  });
  const [period, setPeriod] = useState<Period>("week");
  const [drafts, setDrafts] = useState<Record<string, Draft> | null>(null);
  const [weights, setWeights] = useState<Weight[] | null>(null);
  const current = drafts ?? draftsOf(settings.data?.targets ?? []);
  const currentWeights = weights ?? settings.data?.stageWeights ?? [];
  const people: [string, string][] = [[EVERYONE, "Everyone (unless set below)"], ...(team.data?.rows ?? []).map((r): [string, string] => [r.userId, r.userName])];
  const save = useMutation({
    // Targets of people no longer in the company are dropped.
    mutationFn: async () => unwrap(await api.PUT("/v1/reports/settings", {
      body: { targets: targetsOf(current).filter((t) => t.userId === null || people.some(([id]) => id === t.userId)), stageWeights: currentWeights },
    })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(reportSettingsKey, saved);
      setDrafts(null);
      setWeights(null);
      await queryClient.invalidateQueries({ queryKey: ["reports"] });
    },
  });
  const errors = fieldErrors(save.error);
  const set = (user: string, k: keyof Goal, v: string) => {
    const key = keyOf(user === EVERYONE ? null : user, period);
    setDrafts({ ...current, [key]: { ...(current[key] ?? EMPTY), [k]: v } });
  };
  const setWeight = (i: number, change: Partial<Weight>) => { setWeights(currentWeights.map((w, j) => (j === i ? { ...w, ...change } : w))); };
  // Each problem names whose target (or which stage) it is about.
  const sent = targetsOf(current).filter((t) => t.userId === null || people.some(([id]) => id === t.userId));
  const where = (field: string) => {
    const t = /^targets\.(\d+)\./.exec(field);
    const target = t ? sent[Number(t[1])] : undefined;
    if (target) return `${people.find(([id]) => id === (target.userId ?? EVERYONE))?.[1] ?? "Someone who left"}, per ${target.period}`;
    const w = /^stageWeights\.(\d+)\./.exec(field);
    const weight = w ? currentWeights[Number(w[1])] : undefined;
    return weight ? STAGE[weight.stage] : field;
  };
  const problems = Object.entries(errors).filter(([f]) => f !== "");
  return (
    <main>
      <h2>Targets &amp; forecast</h2>
      <ErrorMessage error={settings.error ?? team.error} />
      {settings.data && (
        <form onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
          <section className="panel" aria-label="Activity targets">
            <div className="row spread">
              <h3>Activity targets</h3>
              <div className="row" role="group" aria-label="Target period">
                {(["week", "month"] as const).map((p) => (
                  <button key={p} type="button" className="choice-pill" aria-pressed={period === p} onClick={() => { setPeriod(p); }}>
                    {p === "week" ? "Per week" : "Per month"}
                  </button>
                ))}
              </div>
            </div>
            <p className="muted small">
              What each person should do {period === "week" ? "each week (Monday to Sunday)" : "each month"}. Leave a box empty for no target.
              A person without their own target has everyone's. Each recruiter sees their progress on the dashboard and the Reports page.
            </p>
            <div className="table-scroll"><table className="targets-table">
              <thead><tr><th>Who</th>{ACTIVITIES.map(([k, label]) => <th key={k}>{label}</th>)}</tr></thead>
              <tbody>
                {people.map(([id, name]) => {
                  const d = current[keyOf(id === EVERYONE ? null : id, period)] ?? EMPTY;
                  return (
                    <tr key={id || "everyone"}>
                      <td>{id === EVERYONE ? <strong>{name}</strong> : name}</td>
                      {ACTIVITIES.map(([k, label]) => (
                        <td key={k}>
                          <input type="number" min={0} max={100000} inputMode="numeric" className="target-input" aria-label={`${name}: ${label} per ${period}`}
                            value={d[k]} onChange={(e) => { set(id, k, e.target.value); }} />
                        </td>
                      ))}
                    </tr>
                  );
                })}
              </tbody>
            </table></div>
          </section>
          <section className="panel" aria-label="Forecast weights">
            <h3>Revenue forecast</h3>
            <p className="muted small">
              How likely a candidate at each stage is to be placed, and when the fee is expected: the forecast counts that share of the fee in that
              month. A placement counts in the month it starts, until it is invoiced. The fee is the one recorded on the placement, else the
              client's terms of business applied to the job's salary budget. Only owners and admins see the forecast.
            </p>
            <div className="table-scroll"><table>
              <thead><tr><th>Stage</th><th>Counts as</th><th>Fee expected</th></tr></thead>
              <tbody>
                {currentWeights.map((w, i) => (
                  <tr key={w.stage}>
                    <td>{STAGE[w.stage]}</td>
                    <td>
                      <input type="number" min={0} max={100} className="target-input" aria-label={`${STAGE[w.stage]}: percent of the fee`}
                        value={w.percent} onChange={(e) => { setWeight(i, { percent: Number(e.target.value) }); }} /> % of the fee
                    </td>
                    <td>
                      {w.stage === "placed" ? <span className="muted">In the month it starts</span> : (
                        <select aria-label={`${STAGE[w.stage]}: month the fee is expected`} value={w.monthsAhead}
                          onChange={(e) => { setWeight(i, { monthsAhead: Number(e.target.value) }); }}>
                          {Array.from({ length: 13 }, (_, n) => <option key={n} value={n}>{n === 0 ? "This month" : n === 1 ? "Next month" : `In ${String(n)} months`}</option>)}
                        </select>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table></div>
          </section>
          {problems.length > 0 && <ul className="error">{problems.map(([f, m]) => <li key={f}>{where(f)}: {m}</li>)}</ul>}
          <div className="row">
            <button type="submit" className="primary" disabled={save.isPending || !team.data || (drafts === null && weights === null)}>Save targets and weights</button>
            {save.isSuccess && <span role="status" className="muted">Saved.</span>}
          </div>
          <ErrorMessage error={problems.length > 0 ? null : save.error} />
        </form>
      )}
    </main>
  );
}
