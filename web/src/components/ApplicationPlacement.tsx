import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationPlacementKey } from "../api/keys";
import { INVOICE_STATUS, PAY_PERIOD, PLACEMENT_JOB_TYPE, PLACEMENT_STATUS, REPLACEMENT, formatDay, formatMoney } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { PlacementForm } from "./PlacementForm";
import { PlacementInvoice } from "./PlacementInvoicing";
import { billingTimeline, trimZeros } from "./placementTimeline";

type Placement = Schemas["Placement"];

/** One line about a placement: job type, start and end, guarantee, PO, what happened, invoice, fee. */
export function PlacementSummary({ p }: { p: Placement }) {
  const fee = p.money?.feeAmount && p.money.feeCurrency ? formatMoney(p.money.feeAmount, p.money.feeCurrency) : null;
  return (
    <>
      <span className={`badge ${p.status}`}>{PLACEMENT_STATUS[p.status]}</span>{" "}
      <span className="muted">
        {PLACEMENT_JOB_TYPE[p.jobType]} · starts {formatDay(p.startDate)}{p.endDate && <> · ends {formatDay(p.endDate)}</>} · guarantee to {formatDay(p.guaranteeEndsOn)}
        {p.poNumber && <> · {p.poNumber}</>}
        {p.leftOn && <> · left {formatDay(p.leftOn)}</>}
        {p.replacement !== "none" && <> · <strong>{REPLACEMENT[p.replacement]}</strong></>}
        {fee && <> · fee {fee}</>} · {INVOICE_STATUS[p.invoiceStatus]}
        {p.recruiterName && <> · {p.recruiterName}</>}
      </span>
    </>
  );
}

/** An application's placement, or recording one (offer and placed stages). */
export function ApplicationPlacement({ applicationId, stage, permissions }: { applicationId: string; stage: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const canManage = permissions.includes("placements:manage");
  const list = useQuery({
    queryKey: applicationPlacementKey(applicationId),
    queryFn: async () => unwrap(await api.GET("/v1/placements", { params: { query: { applicationId } } })).items,
    enabled: stage === "offer" || stage === "placed",
  });
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
  const money = placement.money;
  const fee = money?.feeAmount && money.feeCurrency ? formatMoney(money.feeAmount, money.feeCurrency) : null;
  const salary = money?.salary ? `${formatMoney(money.salary.amount, money.salary.currency)} / ${PAY_PERIOD[money.salary.period].toLowerCase()}` : null;
  const fact = (label: string, value: React.ReactNode) => (
    <div className="placement-fact"><span className="placement-fact-label">{label}</span><span>{value}</span></div>
  );
  const today = new Date().toISOString().slice(0, 10);
  return (
    <div className="placement-box" aria-label="Placement">
      <div className="placement-box-head">
        <span className={`badge ${placement.status}`}>{PLACEMENT_STATUS[placement.status]}</span>
        <strong>Placement</strong>
        {canManage && <button type="button" className="link small" onClick={() => { setEditing(true); }}>Change placement</button>}
      </div>
      <div className="placement-sheet">
        <div className="placement-facts">
          {fact("Job title", placement.jobTitle)}
          {fact("Company", placement.companyName)}
          {fact("Job type", PLACEMENT_JOB_TYPE[placement.jobType])}
          {fact("PO number", placement.poNumber ?? "—")}
          {fact("Start", formatDay(placement.startDate))}
          {fact("End", placement.endDate ? formatDay(placement.endDate) : "—")}
          {salary && fact("Salary", salary)}
          {money?.feePercent && fact("Fee %", `${trimZeros(money.feePercent)}%`)}
          {fee && fact("Fee", <strong>{fee}</strong>)}
          {fact("Guarantee", `${String(placement.guaranteeDays)} days · to ${formatDay(placement.guaranteeEndsOn)}`)}
          {placement.recruiterName && fact("Recruiter", placement.recruiterName)}
          {fact("Invoice", INVOICE_STATUS[placement.invoiceStatus])}
          {placement.leftOn && fact("Left", formatDay(placement.leftOn))}
          {placement.replacement !== "none" && fact("Replacement", REPLACEMENT[placement.replacement])}
        </div>
        <ol className="placement-timeline" aria-label="Billing">
          {billingTimeline(placement, today).map((e) => (
            <li key={`${e.day}-${e.label}`} className={e.state ?? ""}>
              <strong>{formatDay(e.day)}</strong> · {e.label}
              {e.state && e.state !== "done" && <span className={`pill ${e.state}`}>{e.state}</span>}
            </li>
          ))}
        </ol>
      </div>
      <PlacementInvoice p={placement} permissions={permissions} />
    </div>
  );
}
