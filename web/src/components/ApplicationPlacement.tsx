import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationPlacementKey } from "../api/keys";
import { PAY_PERIOD, PLACEMENT_JOB_TYPE, PLACEMENT_STATUS, REPLACEMENT, formatDay, formatMoney } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { PlacementForm } from "./PlacementForm";
import { PlacementInvoice } from "./PlacementInvoicing";
import { billingTimeline, feeNote, invoiceBadge, monthsLong, todayISO, trimZeros } from "./placementTimeline";

type Placement = Schemas["Placement"];

const queued = (list: Placement[] | undefined) => list?.some((p) => p.invoicing.deliveryStatus === "queued") ?? false;

/**
 * An application's placement with its agreed salary (asked for, so counted as shown: read when it is
 * opened, not again when the window gets focus). While its invoice is queued for email, where it stands
 * is read every 5 seconds without the salary, which is kept from the first read.
 */
export function useApplicationPlacement(applicationId: string, enabled: boolean) {
  const full = useQuery({
    queryKey: applicationPlacementKey(applicationId),
    queryFn: async () => unwrap(await api.GET("/v1/placements", { params: { query: { applicationId, includeSalary: true } } })).items,
    refetchOnWindowFocus: false,
    enabled,
  });
  const status = useQuery({
    queryKey: [...applicationPlacementKey(applicationId), "status"],
    queryFn: async () => unwrap(await api.GET("/v1/placements", { params: { query: { applicationId } } })).items,
    enabled: enabled && queued(full.data),
    refetchInterval: (q) => (queued(q.state.data ?? full.data) ? 5000 : false),
    refetchOnWindowFocus: false,
  });
  const data = useMemo(() => {
    if (!full.data || !status.data || status.dataUpdatedAt < full.dataUpdatedAt) return full.data;
    const salaries = new Map(full.data.map((p) => [p.id, p.money?.salary ?? null]));
    return status.data.map((p) => (p.money ? { ...p, money: { ...p.money, salary: salaries.get(p.id) ?? null } } : p));
  }, [full.data, full.dataUpdatedAt, status.data, status.dataUpdatedAt]);
  return { ...full, data };
}

/** An application's placement, or recording one (offer and placed stages). */
export function ApplicationPlacement({ applicationId, stage, permissions }: { applicationId: string; stage: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const canManage = permissions.includes("placements:manage");
  const list = useApplicationPlacement(applicationId, stage === "offer" || stage === "placed");
  const placement = list.data?.[0];
  if (stage !== "offer" && stage !== "placed") return null;
  if (editing) {
    return <PlacementForm applicationId={applicationId} placement={placement} permissions={permissions} onDone={() => { setEditing(false); }} />;
  }
  if (!placement) {
    return (
      <div className="placement">
        <ErrorMessage error={list.error} />
        {canManage && list.isSuccess && (
          <button type="button" className="small-button" onClick={() => { setEditing(true); }}>Record placement</button>
        )}
      </div>
    );
  }
  return <PlacementCard p={placement} permissions={permissions} onEdit={canManage ? () => { setEditing(true); } : undefined} />;
}

/** A placement: what and where, four figures, the billing timeline, details on request, and the invoice. */
function PlacementCard({ p, permissions, onEdit }: { p: Placement; permissions: Permission[]; onEdit?: () => void }) {
  const today = todayISO();
  const money = p.money;
  const fee = money?.feeAmount && money.feeCurrency ? formatMoney(money.feeAmount, money.feeCurrency) : null;
  const badge = invoiceBadge(p, today);
  const figure = (label: string, value: React.ReactNode, note?: string) => (
    <div className="placement-figure">
      <span className="placement-figure-label">{label}</span>
      <strong>{value}</strong>
      {note && <span className="muted small">{note}</span>}
    </div>
  );
  const detail = (label: string, value: React.ReactNode) => <><dt>{label}</dt><dd>{value}</dd></>;
  return (
    <div className="placement-box" aria-label="Placement">
      <div className="placement-head">
        <strong className="placement-title">{p.jobTitle} · {p.companyName}</strong>
        <span className="placement-chip accent">{PLACEMENT_JOB_TYPE[p.jobType]}</span>
        {p.poNumber && <span className="placement-chip">{p.poNumber}</span>}
        {p.status !== "active" && <span className={`badge ${p.status}`}>{PLACEMENT_STATUS[p.status]}</span>}
        {onEdit && <button type="button" className="link small" onClick={onEdit}>Edit</button>}
      </div>
      <div className="placement-figures">
        {fee && money && figure("Fee", fee, feeNote(money))}
        {figure("Dates", p.endDate ? `${formatDay(p.startDate)} – ${formatDay(p.endDate)}` : `From ${formatDay(p.startDate)}`,
          p.endDate ? monthsLong(p.startDate, p.endDate) : undefined)}
        {figure("Guarantee", `to ${formatDay(p.guaranteeEndsOn)}`, `${String(p.guaranteeDays)} days`)}
        {figure("Invoice", <span className={`pill ${badge.state}`}>{badge.label}</span>, badge.note)}
      </div>
      <ol className="placement-steps" aria-label="Billing">
        {billingTimeline(p, today).map((e) => (
          <li key={`${e.day}-${e.label}`} className={e.state}>
            <strong>{formatDay(e.day)}</strong>
            <span className="muted">{e.label}</span>
          </li>
        ))}
      </ol>
      <details className="placement-details">
        <summary>Details</summary>
        <dl>
          {money?.salary && detail("Salary", `${formatMoney(money.salary.amount, money.salary.currency)} / ${PAY_PERIOD[money.salary.period].toLowerCase()}`)}
          {money?.feePercent && detail("Fee %", `${trimZeros(money.feePercent)}%`)}
          {detail("PO number", p.poNumber ?? "—")}
          {detail("Recruiter", p.recruiterName ?? "—")}
          {p.invoiceNumber && detail("Invoice number", p.invoiceNumber)}
          {p.invoicedOn && detail("Invoiced on", formatDay(p.invoicedOn))}
          {p.leftOn && detail("Left", formatDay(p.leftOn))}
          {p.replacement !== "none" && detail("Replacement", REPLACEMENT[p.replacement])}
          {p.statusNote && detail("Note", p.statusNote)}
        </dl>
      </details>
      <PlacementInvoice p={p} permissions={permissions} />
    </div>
  );
}
