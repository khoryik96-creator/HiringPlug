import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { placementsKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { PlacementForm } from "../components/PlacementForm";
import { PlacementInvoice } from "../components/PlacementInvoicing";
import {
  DEFAULT_VIEW, filtered, forCard, GUARANTEE_LABEL, guarantee, KPIS, loadView, onCard, type Range, rangeDates, saveView, SORTS, type View,
} from "../components/placementDashboard";
import { invoiceBadge, localDay } from "../components/placementTimeline";
import { PLACEMENT_JOB_TYPE, PLACEMENT_STATUS, REPLACEMENT, formatDay, formatMoney } from "../labels";

/** Days from today to a date (YYYY-MM-DD); negative once it has passed. */
export function daysUntil(day: string, today = new Date()): number {
  const start = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
  return Math.round((Date.parse(`${day}T00:00:00Z`) - start) / 86_400_000);
}

const RANGES: [Range, string][] = [["this_year", "This year"], ["last_year", "Last year"], ["all", "All time"], ["custom", "Custom"]];
const PAGE = 100;

/**
 * Placements at a glance: by start date, cards that count and filter them
 * (starting soon, in guarantee, guarantee ending, not invoiced, overdue,
 * replacement owed), the fees, and a table with each guarantee's days left.
 */
export function Placements({ permissions }: { permissions: Permission[] }) {
  const [view, setView] = useState<View>(loadView);
  const [editing, setEditing] = useState<string | null>(null);
  const [shown, setShown] = useState(PAGE);
  const [localToday] = useState(() => localDay());
  const dates = rangeDates(view, localToday);
  const customReady = view.range !== "custom" || !dates.from || !dates.to || dates.from <= dates.to;
  const list = useQuery({
    queryKey: [...placementsKey, "dashboard", dates.from, dates.to],
    queryFn: async () => unwrap(await api.GET("/v1/placements", {
      params: { query: { ...(dates.from ? { startFrom: dates.from } : {}), ...(dates.to ? { startTo: dates.to } : {}) } },
    })),
    enabled: customReady,
  });
  useEffect(() => { saveView(view); }, [view]);
  const change = (patch: Partial<View>) => { setView((v) => ({ ...v, ...patch })); setShown(PAGE); };

  const today = list.data?.today ?? localToday; // the agency's day, once loaded
  const items = useMemo(() => list.data?.items ?? [], [list.data]);
  const matching = useMemo(() => filtered(items, view, today), [items, view, today]);
  const rows = forCard(matching, view.kpi, today);
  const recruiters = unique(items.map((p) => [p.recruiterId ?? "", p.recruiterName ?? ""]));
  const companies = unique(items.map((p) => [p.companyId, p.companyName]));
  const seesFees = permissions.includes("placements.fees:read");

  return (
    <main className="placements-dashboard">
      <div className="row spread">
        <h2>Placements</h2>
        <div className="row" role="group" aria-label="Start dates">
          {RANGES.map(([key, label]) => (
            <button key={key} type="button" className="choice-pill" aria-pressed={view.range === key} onClick={() => { change({ range: key }); }}>{label}</button>
          ))}
          {view.range === "custom" && (
            <>
              <input type="date" aria-label="Started from" value={view.from} onChange={(e) => { change({ from: e.target.value }); }} />
              <input type="date" aria-label="Started until" value={view.to} onChange={(e) => { change({ to: e.target.value }); }} />
            </>
          )}
        </div>
      </div>
      {!customReady && <p className="warning" role="alert">The first date is after the second.</p>}
      <ErrorMessage error={list.error} />

      {list.data && (
        <>
          <div className="kpi-cards" role="group" aria-label="Show">
            {KPIS.map((k) => (
              <button key={k.key} type="button" className="kpi-card" aria-pressed={view.kpi === k.key}
                onClick={() => { change({ kpi: view.kpi === k.key ? "all" : k.key }); }}>
                <strong>{matching.filter((p) => onCard(p, k.key, today)).length}</strong>
                <span>{k.label}</span>
                <small>{k.hint}</small>
              </button>
            ))}
          </div>
          {seesFees && list.data.feeTotals && list.data.feeTotals.length > 0 && <Billing totals={list.data.feeTotals} />}
          <Filters view={view} change={change} recruiters={recruiters} companies={companies} />
          <p className="small muted" role="status">
            {rows.length} placement{rows.length === 1 ? "" : "s"} shown
            {dates.from || dates.to ? ` · started ${dates.from ? formatDay(dates.from) : "…"} to ${dates.to ? formatDay(dates.to) : "…"}` : " · all time"}
            {` · today ${formatDay(today)}`}
          </p>
          {list.data.total > items.length && (
            <p className="warning" role="alert">
              Only the newest {items.length} of {list.data.total} placements in these dates are loaded, so the cards and the table count
              those {items.length}. The fees above count all {list.data.total}. Narrow the dates to see the rest.
            </p>
          )}
          {rows.length === 0 ? <p className="muted">{items.length === 0 ? "No placements started in these dates." : "None match."}</p> : (
            <div className="panel table-wrap">
              <table className="placements-table">
                <thead><tr>
                  <th>Candidate</th><th>Job</th><th>Start</th><th>Guarantee</th>{seesFees && <th>Fee</th>}<th>Invoice</th><th>Recruiter</th>
                  <th><span className="visually-hidden">Actions</span></th>
                </tr></thead>
                <tbody>
                  {rows.slice(0, shown).map((p) => editing === p.id ? (
                    <tr key={p.id}><td colSpan={8}>
                      <PlacementForm applicationId={p.applicationId} placement={p} permissions={permissions} onDone={() => { setEditing(null); }} />
                    </td></tr>
                  ) : (
                    <PlacementRow key={p.id} p={p} permissions={permissions} today={today} seesFees={seesFees} onEdit={() => { setEditing(p.id); }} />
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {rows.length > shown && <button type="button" className="doc-btn" onClick={() => { setShown(shown + PAGE); }}>Show more ({rows.length - shown} left)</button>}
        </>
      )}
    </main>
  );
}

/** [id, name] pairs, once each, by name. */
function unique(pairs: string[][]): [string, string][] {
  const seen = new Map<string, string>();
  for (const [id, name] of pairs) if (id && !seen.has(id)) seen.set(id, name ?? id);
  return [...seen.entries()].sort((a, b) => a[1].localeCompare(b[1]));
}

function Filters({ view, change, recruiters, companies }: {
  view: View; change: (p: Partial<View>) => void; recruiters: [string, string][]; companies: [string, string][];
}) {
  return (
    <div className="row placements-filters">
      <input aria-label="Search placements" placeholder="Search candidate, company, job…" value={view.q} onChange={(e) => { change({ q: e.target.value }); }} />
      <select aria-label="Recruiter" value={view.recruiter} onChange={(e) => { change({ recruiter: e.target.value }); }}>
        <option value="">All recruiters</option>
        {recruiters.map(([id, name]) => <option key={id} value={id}>{name}</option>)}
      </select>
      <select aria-label="Company" value={view.company} onChange={(e) => { change({ company: e.target.value }); }}>
        <option value="">All companies</option>
        {companies.map(([id, name]) => <option key={id} value={id}>{name}</option>)}
      </select>
      <select aria-label="Guarantee" value={view.guarantee} onChange={(e) => { change({ guarantee: e.target.value as View["guarantee"] }); }}>
        <option value="">Any guarantee</option>
        {(Object.keys(GUARANTEE_LABEL) as (keyof typeof GUARANTEE_LABEL)[]).map((k) => <option key={k} value={k}>{GUARANTEE_LABEL[k]}</option>)}
      </select>
      <select aria-label="Invoice" value={view.invoice} onChange={(e) => { change({ invoice: e.target.value as View["invoice"] }); }}>
        <option value="">Any invoice</option>
        <option value="not_invoiced">Not invoiced</option><option value="invoiced">Invoiced, not due yet</option>
        <option value="overdue">Overdue</option><option value="paid">Paid</option>
      </select>
      <select aria-label="Sort" value={view.sort} onChange={(e) => { change({ sort: e.target.value as View["sort"] }); }}>
        {SORTS.map(([k, label]) => <option key={k} value={k}>{label}</option>)}
      </select>
      <button type="button" className="link" onClick={() => { change({ ...DEFAULT_VIEW, range: view.range, from: view.from, to: view.to }); }}>Clear filters</button>
    </div>
  );
}

/** The fees of every placement that stood in these dates, per currency. */
function Billing({ totals }: { totals: Schemas["FeeTotal"][] }) {
  return (
    <section className="billing-strip" aria-label="Fees">
      {totals.map((t) => (
        <div key={t.currency} className="billing-row">
          <div><span className="label">Billed</span><strong>{formatMoney(t.total, t.currency)}</strong></div>
          <div><span className="label">Paid</span><strong>{formatMoney(t.paid, t.currency)}</strong></div>
          <div><span className="label">Invoiced, unpaid</span><strong>{formatMoney(t.invoiced, t.currency)}</strong></div>
          <div><span className="label">Not invoiced</span><strong>{formatMoney(t.notInvoiced, t.currency)}</strong></div>
        </div>
      ))}
    </section>
  );
}

/** One placement in the table; its menu has Change and the invoice. */
function PlacementRow({ p, permissions, today, seesFees, onEdit }: {
  p: Schemas["Placement"]; permissions: Permission[]; today: string; seesFees: boolean; onEdit: () => void;
}) {
  const fee = p.money?.feeAmount && p.money.feeCurrency ? formatMoney(p.money.feeAmount, p.money.feeCurrency) : null;
  const badge = invoiceBadge(p, today);
  const g = guarantee(p, today);
  const change = permissions.includes("placements:manage")
    ? (close: () => void) => <button type="button" onClick={() => { close(); onEdit(); }}>Change</button>
    : undefined;
  return (
    <tr>
      <td>
        <a href={`#/candidates/${p.candidateId}`}>{p.candidateName}</a>
        {p.replacement !== "none" && <div className="muted small">{REPLACEMENT[p.replacement]}</div>}
      </td>
      <td><a href={`#/jobs/${p.jobId}`}>{p.jobTitle}</a><div className="muted small">{[p.companyName, PLACEMENT_JOB_TYPE[p.jobType], p.poNumber].filter(Boolean).join(" · ")}</div></td>
      <td className="num">{formatDay(p.startDate)}{p.endDate && <div className="muted small">to {formatDay(p.endDate)}</div>}</td>
      <td>
        <span className={`pill guarantee-${g.state}`}>{p.status === "active" ? GUARANTEE_LABEL[g.state] : PLACEMENT_STATUS[p.status]}</span>
        <div className="muted small">{[g.note, p.status === "active" ? `ends ${formatDay(p.guaranteeEndsOn)}` : ""].filter(Boolean).join(" · ")}</div>
      </td>
      {seesFees && <td className="num">{fee ? <strong>{fee}</strong> : <span className="muted">—</span>}
        {p.money?.retainer?.balance && <div className="muted small">balance {formatMoney(p.money.retainer.balance, p.money.retainer.currency)}</div>}</td>}
      <td><span className={`pill ${badge.state}`}>{badge.label}</span>{badge.note && <div className="muted small">{badge.note}</div>}</td>
      <td className="small">{p.recruiterName ?? <span className="muted">—</span>}</td>
      <td className="actions"><PlacementInvoice p={p} permissions={permissions} variant="menu" extra={change} /></td>
    </tr>
  );
}
