// A daily AI limit (all features together) and each feature's own monthly
// limit, in tokens, besides the month's limit: whichever is reached first
// stops new AI work. Saved with the month's settings as they are.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, unwrap } from "../../api/client";
import { aiUsageStatusKey } from "../../components/AiLimitNotice";
import { ErrorMessage } from "../../components/ErrorMessage";
import { fieldErrors, FormInvalid } from "../../components/forms";
import { allTokens, type Feature, FEATURE } from "./format";
import { aiUsageSettingsKey } from "./LimitForm";
import { bodyOf, draftOf, type Settings } from "./limitFormState";

type Draft = { daily: string; perFeature: Partial<Record<Feature, string>>; version: number };

// Checking an AI provider's key is never held back, so it takes no limit.
const FEATURES = (Object.keys(FEATURE) as Feature[]).filter((f) => f !== "provider_check");

function draftFrom(s: Settings): Draft {
  const perFeature: Partial<Record<Feature, string>> = {};
  for (const [f, n] of Object.entries(s.featureTokens)) perFeature[f as Feature] = allTokens(n);
  return { daily: s.dailyTokens === null ? "" : allTokens(s.dailyTokens), perFeature, version: s.version };
}

/** "120,000" -> 120000; "" -> null; FormInvalid for anything else. */
function tokens(value: string, field: string, problems: Record<string, string>): number | null {
  const v = value.replace(/[\s,]/g, "");
  if (v === "") return null;
  if (!/^[1-9]\d{0,12}$/.test(v)) { problems[field] = "Enter a whole number of tokens above 0, or leave it empty for none."; return null; }
  return Number(v);
}

/** The body to save: the month's settings as saved, with these limits. */
export function limitsBody(s: Settings, d: Draft) {
  const problems: Record<string, string> = {};
  const dailyTokens = tokens(d.daily, "dailyTokens", problems);
  const featureTokens: Record<string, number> = {};
  for (const f of FEATURES) {
    const n = tokens(d.perFeature[f] ?? "", `featureTokens.${f}`, problems);
    if (n !== null) featureTokens[f] = n;
  }
  if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
  return { ...bodyOf({ ...draftOf(s), version: d.version }), dailyTokens, featureTokens };
}

function ReadOnly({ s }: { s: Settings }) {
  const own = FEATURES.filter((f) => s.featureTokens[f] !== undefined);
  return (
    <dl className="ai-usage-values">
      <dt>Daily limit</dt><dd>{s.dailyTokens === null ? "None" : `${allTokens(s.dailyTokens)} tokens a day`}</dd>
      {own.map((f) => <div key={f} className="contents"><dt>{FEATURE[f]}</dt><dd>{allTokens(s.featureTokens[f] ?? 0)} tokens a month</dd></div>)}
    </dl>
  );
}

export function ExtraLimits({ canManage }: { canManage: boolean }) {
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: aiUsageSettingsKey, queryFn: async () => unwrap(await api.GET("/v1/ai-usage/settings")) });
  const [draft, setDraft] = useState<Draft | null>(null);
  const save = useMutation({
    mutationFn: async ({ s, d }: { s: Settings; d: Draft }) => unwrap(await api.PUT("/v1/ai-usage/settings", { body: limitsBody(s, d) })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(aiUsageSettingsKey, saved);
      setDraft(null);
      await Promise.all([queryClient.invalidateQueries({ queryKey: ["ai-usage"] }), queryClient.invalidateQueries({ queryKey: aiUsageStatusKey })]);
    },
    onError: async (err) => {
      if (!(err instanceof ApiError) || err.code !== "CONFLICT_STALE_VERSION") return;
      setDraft(null);
      await queryClient.invalidateQueries({ queryKey: aiUsageSettingsKey });
    },
  });
  const s = settings.data;
  const title = <h3>Daily and per-feature limits</h3>;
  if (!s) return <section className="panel ai-usage-limit" aria-label="Daily and per-feature limits">{title}<ErrorMessage error={settings.error} /></section>;
  if (!canManage) return <section className="panel ai-usage-limit" aria-label="Daily and per-feature limits">{title}<ReadOnly s={s} /></section>;

  const d = draft ?? draftFrom(s);
  const change = (next: Draft) => { setDraft(next); save.reset(); };
  const errors = fieldErrors(save.error);
  const err = (field: string) => errors[field] && <span className="error small">{errors[field]}</span>;
  return (
    <section className="panel ai-usage-limit" aria-label="Daily and per-feature limits">
      {title}
      <p className="muted small">
        Optional, besides the monthly limit: whichever is reached first stops new AI work (CVs are then read by the rules alone).
        Leave a box empty for no limit of its own.
      </p>
      <form onSubmit={(e) => { e.preventDefault(); save.mutate({ s, d }); }}>
        <label className="field ai-daily">Daily limit, all features (tokens)
          <input inputMode="numeric" placeholder="No daily limit" value={d.daily} onChange={(e) => { change({ ...d, daily: e.target.value }); }} />
          {err("dailyTokens")}
        </label>
        <table className="ai-feature-limits">
          <thead><tr><th>Feature</th><th>Monthly limit (tokens)</th></tr></thead>
          <tbody>
            {FEATURES.map((f) => (
              <tr key={f}>
                <td>{FEATURE[f]}</td>
                <td>
                  <input inputMode="numeric" aria-label={`${FEATURE[f]}: monthly limit in tokens`} placeholder="None of its own"
                    value={d.perFeature[f] ?? ""} onChange={(e) => { change({ ...d, perFeature: { ...d.perFeature, [f]: e.target.value } }); }} />
                  {err(`featureTokens.${f}`)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {save.error instanceof ApiError && save.error.code === "CONFLICT_STALE_VERSION"
          ? <p role="alert" className="error small">Someone else changed the AI settings meanwhile; theirs are shown now. Make your change again and save.</p>
          : Object.keys(errors).length === 0 && <ErrorMessage error={save.error} />}
        <div className="row">
          <button type="submit" className="primary" disabled={save.isPending || draft === null}>Save limits</button>
          {save.isSuccess && draft === null && <span role="status" className="muted small">Saved.</span>}
        </div>
      </form>
    </section>
  );
}
