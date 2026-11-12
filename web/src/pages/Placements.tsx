import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { placementsKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { PlacementForm } from "../components/PlacementForm";
import { PlacementInvoice } from "../components/PlacementInvoicing";
import { invoiceBadge, todayISO } from "../components/placementTimeline";
import { INVOICE_STATUS, keysOf, PLACEMENT_JOB_TYPE, PLACEMENT_STATUS, REPLACEMENT, formatDay, formatMoney } from "../labels";

type Filters = { status: Schemas["PlacementStatus"] | ""; invoiceStatus: Schemas["InvoiceStatus"] | "" };

/** Days from today to a date (YYYY-MM-DD); negative once it has passed. */
export function daysUntil(day: string, today = new Date()): number {
  const start = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
  return Math.round((Date.parse(`${day}T00:00:00Z`) - start) / 86_400_000);
}

/** Every placement, with fees by invoice state and guarantees about to end. */
export function Placements({ permissions }: { permissions: Permission[] }) {
  const [filters, setFilters] = useState<Filters>({ status: "", invoiceStatus: "" });
  const [editing, setEditing] = useState<string | null>(null);
  const list = useQuery({
    queryKey: [...placementsKey, filters],
    queryFn: async () => unwrap(await api.GET("/v1/placements", {
      params: { query: { status: filters.status || undefined, invoiceStatus: filters.invoiceStatus || undefined } },
    })),
  });
  const ending = list.data?.items.filter((p) => p.status === "active" && daysUntil(p.guaranteeEndsOn) >= 0 && daysUntil(p.guaranteeEndsOn) <= 14) ?? [];
  const owed = list.data?.items.filter((p) => p.replacement === "owed") ?? [];
  const today = todayISO();
  return (
    <main>
      <h2>Placements</h2>
      <div className="row">
        <label>What happened{" "}
          <select value={filters.status} onChange={(e) => { setFilters({ ...filters, status: e.target.value as Filters["status"] }); }}>
            <option value="">Any</option>
            {keysOf(PLACEMENT_STATUS).map((s) => <option key={s} value={s}>{PLACEMENT_STATUS[s]}</option>)}
          </select>
        </label>
        <label>Invoice{" "}
          <select value={filters.invoiceStatus} onChange={(e) => { setFilters({ ...filters, invoiceStatus: e.target.value as Filters["invoiceStatus"] }); }}>
            <option value="">Any</option>
            {keysOf(INVOICE_STATUS).map((s) => <option key={s} value={s}>{INVOICE_STATUS[s]}</option>)}
          </select>
        </label>
      </div>
      <ErrorMessage error={list.error} />
      {list.data?.feeTotals && list.data.feeTotals.length > 0 && (
        <section className="panel" aria-label="Fees">
          <h3>Fees</h3>
          <table>
            <thead><tr><th>Currency</th><th>Total</th><th>Not invoiced</th><th>Invoiced, unpaid</th><th>Paid</th></tr></thead>
            <tbody>
              {list.data.feeTotals.map((t) => (
                <tr key={t.currency}>
                  <td>{t.currency}</td><td>{formatMoney(t.total, t.currency)}</td><td>{formatMoney(t.notInvoiced, t.currency)}</td>
                  <td>{formatMoney(t.invoiced, t.currency)}</td><td>{formatMoney(t.paid, t.currency)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      {owed.length > 0 && (
        <p className="warning" role="status">Replacement owed to the client: {owed.map((p) => `${p.candidateName} (${p.jobTitle})`).join(", ")}</p>
      )}
      {ending.length > 0 && (
        <p className="warning" role="status">
          Guarantee ending within two weeks: {ending.map((p) => p.candidateName).join(", ")}
        </p>
      )}
      {list.data?.items.length === 0 && <p className="muted">No placements yet.</p>}
      {list.data && list.data.items.length > 0 && (
        <div className="panel table-wrap">
          <table className="placements-table">
            <thead><tr><th>Candidate</th><th>Job</th><th>Type</th><th>Dates</th><th>Fee</th><th>Invoice</th><th><span className="visually-hidden">Actions</span></th></tr></thead>
            <tbody>
              {list.data.items.map((p) => editing === p.id ? (
                <tr key={p.id}><td colSpan={7}>
                  <PlacementForm applicationId={p.applicationId} placement={p} permissions={permissions} onDone={() => { setEditing(null); }} />
                </td></tr>
              ) : (
                <PlacementRow key={p.id} p={p} permissions={permissions} today={today} onEdit={() => { setEditing(p.id); }} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}

/** One placement in the table; its menu has Change and the invoice. */
function PlacementRow({ p, permissions, today, onEdit }: { p: Schemas["Placement"]; permissions: Permission[]; today: string; onEdit: () => void }) {
  const fee = p.money?.feeAmount && p.money.feeCurrency ? formatMoney(p.money.feeAmount, p.money.feeCurrency) : null;
  const badge = invoiceBadge(p, today);
  const change = permissions.includes("placements:manage")
    ? (close: () => void) => <button type="button" onClick={() => { close(); onEdit(); }}>Change</button>
    : undefined;
  return (
    <tr>
      <td>
        <a href={`#/candidates/${p.candidateId}`}>{p.candidateName}</a>
        {p.status !== "active" && <div><span className={`badge ${p.status}`}>{PLACEMENT_STATUS[p.status]}</span></div>}
        {p.replacement !== "none" && <div className="muted small">{REPLACEMENT[p.replacement]}</div>}
      </td>
      <td><a href={`#/jobs/${p.jobId}`}>{p.jobTitle}</a><div className="muted small">{p.companyName}</div></td>
      <td>{PLACEMENT_JOB_TYPE[p.jobType]}{p.poNumber && <div className="muted small">{p.poNumber}</div>}</td>
      <td className="num">{formatDay(p.startDate)}{p.endDate && <div className="muted small">to {formatDay(p.endDate)}</div>}</td>
      <td className="num">{fee ? <strong>{fee}</strong> : <span className="muted">—</span>}</td>
      <td><span className={`pill ${badge.state}`}>{badge.label}</span>{badge.note && <div className="muted small">{badge.note}</div>}</td>
      <td className="actions"><PlacementInvoice p={p} permissions={permissions} variant="menu" extra={change} /></td>
    </tr>
  );
}
