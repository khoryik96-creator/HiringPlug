// The company's monthly AI limit (tokens or money), the warning level and
// the prices per million tokens: a form for those who manage them, the values
// for those who only read them. A save over someone else's newer settings
// (judged by the version the edit started from) reloads theirs and says so.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, unwrap } from "../../api/client";
import { aiUsageStatusKey } from "../../components/AiLimitNotice";
import { ErrorMessage } from "../../components/ErrorMessage";
import { CURRENCIES, fieldErrors } from "../../components/forms";
import { aiMoney, allTokens } from "./format";
import { bodyOf, type Draft, draftOf, hiddenField, serverDefault, type Settings } from "./limitFormState";

export const aiUsageSettingsKey = ["ai-usage", "settings"] as const;

function ReadOnly({ s }: { s: Settings }) {
  const prices = s.priceInputPerMillion !== null && s.priceOutputPerMillion !== null
    ? `${aiMoney(s.priceInputPerMillion, s.currency)} in, ${aiMoney(s.priceOutputPerMillion, s.currency)} out`
    : "Not set";
  const limit = s.limitKind === "money" && s.limitMoney !== null
    ? `${aiMoney(s.limitMoney, s.currency)} a month`
    : `${allTokens(s.limitTokens ?? s.serverDefaultTokens)} tokens a month${s.limitTokens === null ? " (the server's default)" : ""}`;
  return (
    <>
      <dl className="ai-usage-values">
        <dt>Monthly limit</dt><dd>{limit}</dd>
        <dt>Warn at</dt><dd>{s.warnPercent}%</dd>
        <dt>Price per 1M tokens</dt><dd>{prices}</dd>
      </dl>
      <p className="muted small">Owners and admins change the limit and the prices.</p>
    </>
  );
}

export function LimitForm({ canManage }: { canManage: boolean }) {
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: aiUsageSettingsKey, queryFn: async () => unwrap(await api.GET("/v1/ai-usage/settings")) });
  const [draft, setDraft] = useState<Draft | null>(null);
  const [reloaded, setReloaded] = useState(false);
  const save = useMutation({
    mutationFn: async (d: Draft) => unwrap(await api.PUT("/v1/ai-usage/settings", { body: bodyOf(d) })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(aiUsageSettingsKey, saved);
      setDraft(null);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["ai-usage"] }),
        queryClient.invalidateQueries({ queryKey: aiUsageStatusKey }),
      ]);
    },
    onError: async (err) => {
      if (!(err instanceof ApiError) || err.code !== "CONFLICT_STALE_VERSION") return;
      setDraft(null);
      setReloaded(true);
      // Their limit may differ from the one this month's figures, the chart and the banner were drawn with.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["ai-usage"] }),
        queryClient.invalidateQueries({ queryKey: aiUsageStatusKey }),
      ]);
    },
  });
  const s = settings.data;
  if (!s) return <section className="panel ai-usage-limit" aria-label="Limit and prices"><h3>Limit and prices</h3><ErrorMessage error={settings.error} /></section>;
  if (!canManage) return <section className="panel ai-usage-limit" aria-label="Limit and prices"><h3>Limit and prices</h3><ReadOnly s={s} /></section>;

  const d = draft ?? draftOf(s);
  const set = (change: Partial<Draft>) => { setDraft({ ...d, ...change }); setReloaded(false); save.reset(); };
  const errors = fieldErrors(save.error);
  const stale = save.error instanceof ApiError && save.error.code === "CONFLICT_STALE_VERSION";
  const hidden = hiddenField(d);
  const shown = ["limitKind", "limitTokens", "limitMoney", "currency", "priceInputPerMillion", "priceOutputPerMillion", "warnPercent"]
    .filter((f) => f !== hidden.field);
  const other = Object.entries(errors).filter(([f]) => !shown.includes(f))
    .map(([f, m]) => [f, f === hidden.field ? hidden.label : f, m] as const);
  const currencies = CURRENCIES.includes(d.currency) ? CURRENCIES : [d.currency, ...CURRENCIES];
  const err = (field: string) => errors[field] && <span className="error small">{errors[field]}</span>;
  const pricesMissing = d.kind === "money" && (d.priceIn.trim() === "" || d.priceOut.trim() === "");
  return (
    <section className="panel ai-usage-limit" aria-label="Limit and prices">
      <h3>Limit and prices</h3>
      <form onSubmit={(e) => { e.preventDefault(); save.mutate(d); }}>
        <div className="ai-usage-fields">
          <label className="field">Limit in
            <select value={d.kind} onChange={(e) => { set({ kind: e.target.value === "money" ? "money" : "tokens" }); }}>
              <option value="tokens">Tokens</option>
              <option value="money">Money</option>
            </select>
            {err("limitKind")}
          </label>
          {d.kind === "tokens" ? (
            <label className="field">Monthly limit (tokens)
              <input inputMode="numeric" value={d.useDefault ? allTokens(s.serverDefaultTokens) : d.tokens} disabled={d.useDefault}
                onChange={(e) => { set({ tokens: e.target.value }); }} />
              {err("limitTokens")}
            </label>
          ) : (
            <label className="field">Monthly limit ({d.currency})
              <input inputMode="decimal" placeholder="e.g. 50.00" value={d.money} onChange={(e) => { set({ money: e.target.value }); }} />
              {err("limitMoney")}
            </label>
          )}
          {d.kind === "tokens" && (
            <label className="check full">
              <input type="checkbox" checked={d.useDefault} onChange={(e) => { set({ useDefault: e.target.checked }); }} />
              Use the server default ({allTokens(s.serverDefaultTokens)} tokens)
            </label>
          )}
          <label className="field">Warn at (%)
            <input inputMode="numeric" value={d.warn} onChange={(e) => { set({ warn: e.target.value }); }} />
            {err("warnPercent")}
          </label>
          <label className="field">Currency
            <select value={d.currency} onChange={(e) => { set({ currency: e.target.value }); }}>
              {currencies.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
            {err("currency")}
          </label>
          <label className="field">Price per 1M tokens in
            <input inputMode="decimal" placeholder="e.g. 0.27" value={d.priceIn} onChange={(e) => { set({ priceIn: e.target.value }); }} />
            {err("priceInputPerMillion")}
          </label>
          <label className="field">Price per 1M tokens out
            <input inputMode="decimal" placeholder="e.g. 1.10" value={d.priceOut} onChange={(e) => { set({ priceOut: e.target.value }); }} />
            {err("priceOutputPerMillion")}
          </label>
        </div>
        <p className="muted small">
          {pricesMissing ? "A limit in money needs both prices. " : ""}Prices are on your AI provider&apos;s price page. Tokens in are what is sent to the AI; tokens out are what it writes.
        </p>
        {other.length > 0 && <ul role="alert" className="error small">{other.map(([f, label, m]) => <li key={f}>{label}: {m}</li>)}</ul>}
        {reloaded && <p role="alert" className="error small">Someone else changed these settings while you were editing, so their version is shown now. Make your change again and save.</p>}
        {!stale && Object.keys(errors).length === 0 && <ErrorMessage error={save.error} />}
        <div className="row">
          <button type="submit" className="primary" disabled={save.isPending || draft === null}>Save</button>
          {d.kind === "money" && (
            <button type="button" className="link" onClick={() => { set(serverDefault(d, s.serverDefaultTokens)); }}>Use the server default</button>
          )}
          {save.isSuccess && draft === null && <span role="status" className="muted small">Saved.</span>}
        </div>
      </form>
    </section>
  );
}
