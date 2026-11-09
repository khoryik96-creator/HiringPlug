import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationPlacementKey } from "../api/keys";
import { INVOICE_STATUS, PLACEMENT_STATUS, REPLACEMENT, formatDay, formatMoney } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { PlacementForm } from "./PlacementForm";
import { PlacementInvoice } from "./PlacementInvoicing";

type Placement = Schemas["Placement"];

/** One line about a placement: started, guarantee, what happened, invoice, fee. */
export function PlacementSummary({ p }: { p: Placement }) {
  const fee = p.money?.feeAmount && p.money.feeCurrency ? formatMoney(p.money.feeAmount, p.money.feeCurrency) : null;
  return (
    <>
      <span className={`badge ${p.status}`}>{PLACEMENT_STATUS[p.status]}</span>{" "}
      <span className="muted">
        starts {formatDay(p.startDate)} · guarantee to {formatDay(p.guaranteeEndsOn)}
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
  const fee = placement.money?.feeAmount && placement.money.feeCurrency ? formatMoney(placement.money.feeAmount, placement.money.feeCurrency) : null;
  const fact = (label: string, value: React.ReactNode) => (
    <div className="placement-fact"><span className="placement-fact-label">{label}</span><span>{value}</span></div>
  );
  return (
    <div className="placement-box" aria-label="Placement">
      <div className="placement-box-head">
        <span className={`badge ${placement.status}`}>{PLACEMENT_STATUS[placement.status]}</span>
        <strong>Placement</strong>
        {canManage && <button type="button" className="link small" onClick={() => { setEditing(true); }}>Change placement</button>}
      </div>
      <div className="placement-facts">
        {fact("Starts", formatDay(placement.startDate))}
        {fact("Guarantee to", formatDay(placement.guaranteeEndsOn))}
        {fee && fact("Fee", <strong>{fee}</strong>)}
        {fact("Invoice", INVOICE_STATUS[placement.invoiceStatus])}
        {placement.recruiterName && fact("Recruiter", placement.recruiterName)}
        {placement.leftOn && fact("Left", formatDay(placement.leftOn))}
        {placement.replacement !== "none" && fact("Replacement", REPLACEMENT[placement.replacement])}
      </div>
      <PlacementInvoice p={placement} permissions={permissions} />
    </div>
  );
}
