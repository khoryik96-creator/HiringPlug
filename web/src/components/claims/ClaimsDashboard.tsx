// The claims dashboard: what is waiting, approved and paid, and where the money
// went (by receipt type, claim type, client or candidate, person and month).
// Each breakdown is a bar list (a name, a bar, the figures), read as a table.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../../api/client";
import { claimsKey } from "../../api/keys";
import { formatMoney } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { useFieldSettings } from "../useFieldSettings";
import { breakdown, CLAIMED, type ClaimStatus, currenciesOf, type Fact, fromCents, type Period, PERIODS, periodDates, type Row, statusTotal } from "./claims";

const TILES: [ClaimStatus, string, string][] = [
  ["submitted", "Waiting for approval", "submitted, not decided yet"],
  ["approved", "Approved, to pay", "approved, not paid yet"],
  ["paid", "Paid", "receipts in this period"],
  ["rejected", "Rejected", "to fix and send again"],
];

export function ClaimsDashboard({ canSeeTeam }: { canSeeTeam: boolean }) {
  const fs = useFieldSettings();
  const [team, setTeam] = useState(canSeeTeam);
  const [period, setPeriod] = useState<Period>("this_month");
  const [chosenCurrency, setCurrency] = useState("");
  const { from, to } = periodDates(period);
  const summary = useQuery({
    queryKey: [...claimsKey, "summary", team, from, to],
    queryFn: async () => unwrap(await api.GET(team ? "/v1/claims/team/summary" : "/v1/claims/summary", { params: { query: { from, to } } })),
  });
  const facts = summary.data?.facts ?? [];
  const currencies = currenciesOf(facts);
  const currency = currencies.includes(chosenCurrency) ? chosenCurrency : (currencies[0] ?? "MYR");
  const names = new Map((summary.data?.people ?? []).map((p) => [p.id, p.name]));
  const records = new Map((summary.data?.records ?? []).map((r) => [`${r.kind}:${r.id}`, r.name ? (r.detail ? `${r.name} (${r.detail})` : r.name) : `A ${r.kind} you cannot see`]));
  const drafts = statusTotal(facts, currency, "draft");
  const claimed = breakdown(facts, currency, () => "all")[0];

  return (
    <section className="claims-dashboard" aria-label="Claims dashboard">
      <div className="row claim-filters">
        <div className="row" role="group" aria-label="Period">
          {PERIODS.map(([key, label]) => (
            <button key={key} type="button" className="choice-pill" aria-pressed={period === key} onClick={() => { setPeriod(key); }}>{label}</button>
          ))}
        </div>
        {canSeeTeam && (
          <div className="row" role="group" aria-label="Whose claims">
            <button type="button" className="choice-pill" aria-pressed={team} onClick={() => { setTeam(true); }}>Team</button>
            <button type="button" className="choice-pill" aria-pressed={!team} onClick={() => { setTeam(false); }}>Mine</button>
          </div>
        )}
        {currencies.length > 1 && (
          <select aria-label="Currency" value={currency} onChange={(e) => { setCurrency(e.target.value); }}>
            {currencies.map((c) => <option key={c}>{c}</option>)}
          </select>
        )}
      </div>
      <ErrorMessage error={summary.error} />
      {summary.data && (
        <>
          <div className="kpi-cards claim-tiles">
            {TILES.map(([status, label, hint]) => {
              const t = statusTotal(facts, currency, status);
              return (
                <div key={status} className={`kpi-card claim-tile ${status}`}>
                  <span>{label}</span>
                  <strong>{formatMoney(fromCents(t.cents), currency)}</strong>
                  <small>{t.count} claim{t.count === 1 ? "" : "s"} · {hint}</small>
                </div>
              );
            })}
            {!team && (
              <div className="kpi-card claim-tile draft">
                <span>Drafts</span>
                <strong>{drafts.count}</strong>
                <small>receipts not submitted yet</small>
              </div>
            )}
          </div>
          <p className="small muted">
            Claimed (waiting, approved or paid) with receipts dated {from} to {to}: <strong>{formatMoney(fromCents(claimed?.cents ?? 0), currency)}</strong>
            {claimed ? ` in ${String(claimed.count)} claim${claimed.count === 1 ? "" : "s"}` : ""}.
          </p>
          <div className="claim-breakdowns">
            <BarList title="By receipt type" currency={currency} rows={breakdown(facts, currency, (f) => f.receiptType)}
              name={(k) => (k ? fs.label("claim_receipt_type", k) : "Not set")} swatch={(k) => (k ? fs.color("claim_receipt_type", k) : undefined)} />
            <BarList title="By claim type" currency={currency} rows={breakdown(facts, currency, (f) => f.claimType)}
              name={(k) => (k ? fs.label("claim_type", k) : "Not set")} />
            {team && <BarList title="By person" currency={currency} rows={breakdown(facts, currency, (f) => f.userId)} name={(k) => names.get(k) ?? "Someone"} />}
            <BarList title="By client or candidate" currency={currency} rows={top(breakdown(facts, currency, linkKey), 8)}
              name={(k) => (k === OTHER ? "All others" : k ? records.get(k) ?? "—" : "Not tagged")} />
            <BarList title="By month" currency={currency} rows={byMonth(facts, currency)} name={monthWords} ordered />
          </div>
        </>
      )}
    </section>
  );
}

const OTHER = "\u0000other";
const linkKey = (f: Fact) => (f.linkKind && f.linkId ? `${f.linkKind}:${f.linkId}` : "");

/** The first n rows, the rest folded into one "All others" row. */
function top(rows: Row[], n: number): Row[] {
  if (rows.length <= n) return rows;
  const rest = rows.slice(n - 1);
  return [...rows.slice(0, n - 1), { key: OTHER, cents: rest.reduce((s, r) => s + r.cents, 0), count: rest.reduce((s, r) => s + r.count, 0) }];
}

/** Months in order (a month with nothing claimed is left out). */
function byMonth(facts: Fact[], currency: string): Row[] {
  return breakdown(facts, currency, (f) => f.month, CLAIMED).sort((a, b) => a.key.localeCompare(b.key));
}

function monthWords(month: string): string {
  return new Date(`${month}-01T00:00:00Z`).toLocaleDateString("en-GB", { month: "short", year: "numeric", timeZone: "UTC" });
}

/** A breakdown: per row a name, a bar (one colour; its length the amount) and the figures. */
function BarList({ title, rows, currency, name, swatch, ordered = false }: {
  title: string; rows: Row[]; currency: string; name: (key: string) => string; swatch?: (key: string) => string | undefined; ordered?: boolean;
}) {
  const max = Math.max(1, ...rows.map((r) => r.cents));
  const sum = rows.reduce((s, r) => s + r.cents, 0);
  return (
    <section className="panel claim-bars" aria-label={title}>
      <h4>{title}</h4>
      {rows.length === 0 ? <p className="muted small">Nothing claimed in this period.</p> : (
        <table>
          <thead className="visually-hidden"><tr><th>{ordered ? "Month" : "Name"}</th><th>Share</th><th>Amount</th></tr></thead>
          <tbody>
            {rows.map((r) => {
              const color = swatch?.(r.key);
              const share = sum ? Math.round((r.cents / sum) * 100) : 0;
              const words = `${formatMoney(fromCents(r.cents), currency)} · ${String(r.count)} claim${r.count === 1 ? "" : "s"} · ${String(share)}%`;
              return (
                <tr key={r.key || "none"}>
                  <th scope="row">{color && <span className="swatch" style={{ background: color }} aria-hidden="true" />}{name(r.key)}</th>
                  <td className="bar-cell" title={words}>
                    <span className="bar" style={{ width: `${String(Math.max(1.5, (r.cents / max) * 100))}%` }} />
                    <span className="visually-hidden">{share}%</span>
                  </td>
                  <td className="num">{formatMoney(fromCents(r.cents), currency)} <small className="muted">{r.count} claim{r.count === 1 ? "" : "s"}</small></td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </section>
  );
}
