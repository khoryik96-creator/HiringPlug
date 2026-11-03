import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { placementsKey } from "../api/keys";
import { PlacementSummary } from "../components/ApplicationPlacement";
import { ErrorMessage } from "../components/ErrorMessage";
import { PlacementForm } from "../components/PlacementForm";
import { INVOICE_STATUS, keysOf, PLACEMENT_STATUS, formatMoney } from "../labels";

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
  return (
    <>
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
          <p className="muted">Placements that fell through or did not start are not counted.</p>
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
      {list.data?.items.length === 0 && <p className="muted">No placements yet. Record one from a candidate&apos;s job at the offer stage.</p>}
      {list.data?.items.map((p) => editing === p.id ? (
        <PlacementForm key={p.id} applicationId={p.applicationId} placement={p} permissions={permissions} onDone={() => { setEditing(null); }} />
      ) : (
        <article key={p.id} className="panel" aria-label={`${p.candidateName}, ${p.jobTitle}`}>
          <div className="row spread">
            <div>
              <a href={`#/candidates/${p.candidateId}`}><strong>{p.candidateName}</strong></a> as{" "}
              <a href={`#/jobs/${p.jobId}`}>{p.jobTitle}</a> <span className="muted">at {p.companyName}</span>
              <div><PlacementSummary p={p} /></div>
              {p.statusNote && <div className="muted">{p.statusNote}</div>}
            </div>
            {permissions.includes("placements:manage") && (
              <button type="button" className="link" onClick={() => { setEditing(p.id); }}>Change</button>
            )}
          </div>
        </article>
      ))}
    </>
  );
}
