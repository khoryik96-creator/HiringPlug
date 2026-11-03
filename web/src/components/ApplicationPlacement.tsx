import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationPlacementKey } from "../api/keys";
import { INVOICE_STATUS, PLACEMENT_STATUS, REPLACEMENT, formatDay, formatMoney } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { PlacementForm } from "./PlacementForm";

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
  return (
    <div className="placement">
      <ErrorMessage error={list.error} />
      {placement && <PlacementSummary p={placement} />}{" "}
      {canManage && list.isSuccess && (
        <button type="button" className="link" onClick={() => { setEditing(true); }}>
          {placement ? "Change placement" : "Record placement"}
        </button>
      )}
    </div>
  );
}
