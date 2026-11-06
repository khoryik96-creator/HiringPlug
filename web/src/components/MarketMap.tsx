// A job's market map: who in the database does this kind of work (how many,
// how available, where they work and live, work authorization, notice, pay),
// the AI's market insights (like Kano's Owl), and the map as Word or PDF.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { assistantKey } from "../api/keys";
import { PAY_PERIOD } from "../labels";
import { AdText } from "./AdText";
import { AVAILABILITY } from "./candidateLook";
import { ErrorMessage } from "./ErrorMessage";

type Count = Schemas["MarketCount"];
type Pay = Schemas["MarketPay"];

const marketKey = (jobId: string) => ["market-map", jobId] as const;
const STATUS_LABEL: Record<string, string> = { ...Object.fromEntries(Object.entries(AVAILABILITY).map(([k, v]) => [k, v.label])), unknown: "Not set" };
const AUTH: Record<string, { label: string; color: string }> = {
  local: { label: "Local", color: "#15803D" }, pr: { label: "PR", color: "#7E22CE" }, expat: { label: "Spousal / RP-T", color: "#1D4ED8" },
  needs_pass: { label: "Needs pass", color: "#B91C1C" }, unknown: { label: "Not known", color: "#6B7280" },
};
const TYPES = { docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdf: "application/pdf" } as const;

const money = (currency: string, amount: string) =>
  `${currency === "MYR" ? "RM" : currency} ${Number(amount).toLocaleString("en-MY", { maximumFractionDigits: 0 })}`;
const countOf = (cs: Count[], key: string) => cs.find((c) => c.key === key)?.count ?? 0;

export function MarketMap({ jobId, permissions }: { jobId: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const allowed = permissions.includes("market:read");
  const map = useQuery({
    queryKey: marketKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/market-map", { params: { path: { jobId } } })),
    enabled: allowed,
    // Follow insights the AI is still writing.
    refetchInterval: (q) => (q.state.data?.insights?.status === "queued" ? 3000 : false),
  });
  const canAsk = permissions.includes("assistant:use");
  const ai = useQuery({ queryKey: assistantKey, queryFn: async () => unwrap(await api.GET("/v1/assistant")), enabled: allowed && canAsk });
  const ask = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/assistant/answers", { body: { subjectType: "job", subjectId: jobId, kind: "market_map" } })),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: marketKey(jobId) }); },
  });
  const [exportError, setExportError] = useState<unknown>(null);
  if (!allowed) return null;
  const m = map.data;
  const exportAs = (format: keyof typeof TYPES) => {
    setExportError(null);
    downloadFrom(`/v1/jobs/${jobId}/market-map/export?format=${format}`, `Market map - ${m?.jobTitle ?? "job"}.${format}`, TYPES[format])
      .catch((e: unknown) => { setExportError(e); });
  };

  return (
    <section className="panel market" aria-label="Market map">
      <div className="row spread">
        <h3>Market map</h3>
        <span className="row">
          <button type="button" className="link" onClick={() => { exportAs("docx"); }}>Word</button>
          <button type="button" className="link" onClick={() => { exportAs("pdf"); }}>PDF</button>
        </span>
      </div>
      <ErrorMessage error={map.error ?? ask.error ?? exportError} />
      {m && (
        <>
          <p className="muted small">In your database: current title with “{m.searchedFor}”</p>
          <div className="market-stats">
            <div><strong>{m.pool.total}</strong><span>candidates</span></div>
            <div><strong>{countOf(m.pool.availability, "looking")}</strong><span>looking out</span></div>
            <div><strong>{countOf(m.pool.availability, "passive")}</strong><span>passive</span></div>
            <div><strong>{m.pool.medianNoticeDays ?? "–"}</strong><span>median notice (days)</span></div>
          </div>
          {m.pool.total > 0 && (
            <div className="market-grid">
              <Bars title="Where they work" counts={m.pool.companies} />
              <Bars title="Where they live" counts={m.pool.locations} />
              <div>
                <h4>Work authorization</h4>
                <ul className="market-auth">
                  {m.pool.workAuthorization.map((c) => (
                    <li key={c.key}><span className="dot square" style={{ background: AUTH[c.key]?.color }} />{AUTH[c.key]?.label ?? c.key}<strong>{c.count}</strong></li>
                  ))}
                </ul>
                <h4>Status</h4>
                <ul className="market-auth">
                  {m.pool.availability.map((c) => <li key={c.key}>{STATUS_LABEL[c.key] ?? c.key}<strong>{c.count}</strong></li>)}
                </ul>
              </div>
            </div>
          )}
          {m.pool.salaryRestricted ? <p className="muted small">🔒 Pay is restricted for your role.</p> : m.pool.salary.length > 0 && <PayTable pay={m.pool.salary} />}
          <Insights insights={m.insights} enabled={ai.data?.enabled === true} canAsk={canAsk} asking={ask.isPending} onAsk={() => { ask.mutate(); }} />
        </>
      )}
    </section>
  );
}

function Bars({ title, counts }: { title: string; counts: Count[] }) {
  const most = Math.max(1, ...counts.map((c) => c.count));
  return (
    <div>
      <h4>{title}</h4>
      {counts.length === 0 ? <p className="muted small">Not recorded.</p> : (
        <ul className="market-bars" aria-label={title}>
          {counts.map((c) => (
            <li key={c.key}>
              <span className="market-bar-label">{c.key}</span>
              <span className="market-bar"><span style={{ width: `${String((c.count / most) * 100)}%` }} /></span>
              <strong>{c.count}</strong>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function PayTable({ pay }: { pay: Pay[] }) {
  return (
    <table className="market-pay" aria-label="Pay">
      <thead><tr><th>Pay</th><th>Low (25%)</th><th>Median</th><th>High (75%)</th><th>Candidates</th></tr></thead>
      <tbody>
        {pay.flatMap((g) => (["current", "expected"] as const).map((k) => {
          const r = g[k];
          if (!r) return null;
          return (
            <tr key={`${g.currency}-${g.period}-${k}`}>
              <td>{k === "current" ? "Current" : "Expected"} <span className="muted">/ {PAY_PERIOD[g.period]}</span></td>
              <td>{money(g.currency, r.low)}</td><td><strong>{money(g.currency, r.median)}</strong></td><td>{money(g.currency, r.high)}</td><td>{r.count}</td>
            </tr>
          );
        }))}
      </tbody>
    </table>
  );
}

function Insights({ insights, enabled, canAsk, asking, onAsk }: {
  insights: Schemas["MarketInsights"] | null; enabled: boolean; canAsk: boolean; asking: boolean; onAsk: () => void;
}) {
  const queued = insights?.status === "queued";
  return (
    <div className="market-insights">
      <div className="row spread">
        <h4>Market insights (AI)</h4>
        {canAsk && enabled && (
          <button type="button" className="primary" disabled={asking || queued} onClick={onAsk}>{insights ? "Refresh insights" : "Get insights"}</button>
        )}
      </div>
      {!enabled && <p className="muted small">Off: no AI provider is set up.</p>}
      {queued && <p className="muted small" role="status">Writing…</p>}
      {insights?.status === "failed" && <p className="warning small">No insights this time: try again.</p>}
      {insights?.status === "done" && insights.text && (
        <>
          <AdText text={insights.text} />
          <p className="muted small">AI estimates from its knowledge of the market: check before relying on them.</p>
        </>
      )}
    </div>
  );
}
