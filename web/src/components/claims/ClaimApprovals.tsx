// Everyone's claims, for approvers: approve or reject what is waiting (with a
// reason), mark approved ones paid, see each receipt, download a CSV.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { claimsKey } from "../../api/keys";
import { formatDay, formatMoney } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { FileViewer } from "../FileViewer";
import { localDay } from "../placementTimeline";
import { useFieldSettings } from "../useFieldSettings";
import { type Claim, CLAIM_STATUS, type ClaimStatus, claimsCsv, formatTotals, totals } from "./claims";
import { linkWords } from "./LinkPicker";
import { TypeChip } from "./MyClaims";
import { UploadNote } from "./UploadNote";

const VIEWS: [ClaimStatus | "", string][] = [
  ["submitted", "Waiting for approval"], ["approved", "Approved, to pay"], ["paid", "Paid"], ["rejected", "Rejected"], ["", "All"],
];

export function ClaimApprovals() {
  const client = useQueryClient();
  const fs = useFieldSettings();
  const [status, setStatus] = useState<ClaimStatus | "">("submitted");
  const [person, setPerson] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [rejecting, setRejecting] = useState(false);
  const [note, setNote] = useState("");
  const [viewing, setViewing] = useState<Claim | null>(null);
  const query = {
    limit: 500, ...(status ? { status } : {}), ...(person ? { userId: person } : {}), ...(from ? { from } : {}), ...(to ? { to } : {}),
  };
  const list = useQuery({
    queryKey: [...claimsKey, "team", query],
    queryFn: async () => unwrap(await api.GET("/v1/claims/team", { params: { query } })),
  });
  const people = useQuery({
    queryKey: [...claimsKey, "team", "people"],
    queryFn: async () => unwrap(await api.GET("/v1/claims/team/summary")).people,
  });
  const review = useMutation({
    mutationFn: async ({ claims, decision }: { claims: Claim[]; decision: Schemas["ClaimDecision"] }) => unwrap(await api.POST("/v1/claims/review", {
      body: { claims: claims.map((c) => ({ id: c.id, version: c.version })), decision, ...(decision === "reject" ? { note } : {}) },
    })),
    onSuccess: async () => {
      setSelected(new Set());
      setRejecting(false);
      setNote("");
      await client.invalidateQueries({ queryKey: claimsKey });
    },
  });

  const items = list.data?.items ?? [];
  const chosen = items.filter((c) => selected.has(c.id));
  const waiting = chosen.filter((c) => c.status === "submitted");
  const approved = chosen.filter((c) => c.status === "approved");
  const filter = (f: () => void) => { f(); setSelected(new Set()); };
  const download = () => {
    const csv = claimsCsv(items, { receiptType: (k) => fs.label("claim_receipt_type", k), claimType: (k) => fs.label("claim_type", k) });
    const a = Object.assign(document.createElement("a"), {
      href: URL.createObjectURL(new Blob([csv], { type: "text/csv" })), download: `claims-${localDay()}.csv`,
    });
    a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); }, 10_000);
  };

  return (
    <section className="claim-approvals" aria-label="Approvals">
      <div className="row claim-filters">
        <div className="row" role="group" aria-label="Status">
          {VIEWS.map(([key, label]) => (
            <button key={key || "all"} type="button" className="choice-pill" aria-pressed={status === key} onClick={() => { filter(() => { setStatus(key); }); }}>{label}</button>
          ))}
        </div>
        <select aria-label="Person" value={person} onChange={(e) => { filter(() => { setPerson(e.target.value); }); }}>
          <option value="">Everyone</option>
          {(people.data ?? []).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
        <input type="date" aria-label="Receipts from" value={from} onChange={(e) => { filter(() => { setFrom(e.target.value); }); }} />
        <input type="date" aria-label="Receipts until" value={to} onChange={(e) => { filter(() => { setTo(e.target.value); }); }} />
        <button type="button" className="link" disabled={items.length === 0} onClick={download}>Download CSV</button>
      </div>
      <ErrorMessage error={list.error ?? review.error} />
      {list.data && (
        <p className="small muted" role="status">
          {list.data.total} claim{list.data.total === 1 ? "" : "s"} · {formatTotals(list.data.totals)}
          {list.data.total > items.length ? ` (showing the newest ${String(items.length)})` : ""}
        </p>
      )}

      {chosen.length > 0 && (
        <div className="claim-actions panel" role="group" aria-label="Selected claims">
          <strong>{chosen.length} selected · {formatTotals(totals(chosen))}</strong>
          <button type="button" className="primary" disabled={waiting.length === 0 || review.isPending}
            onClick={() => { review.mutate({ claims: waiting, decision: "approve" }); }}>Approve {waiting.length || ""}</button>
          <button type="button" disabled={waiting.length === 0 || review.isPending} onClick={() => { setRejecting(true); }}>Reject…</button>
          <button type="button" disabled={approved.length === 0 || review.isPending}
            onClick={() => { review.mutate({ claims: approved, decision: "pay" }); }}>Mark {approved.length || ""} paid</button>
          {rejecting && (
            <form className="row claim-reject" onSubmit={(e) => { e.preventDefault(); review.mutate({ claims: waiting, decision: "reject" }); }}>
              <input aria-label="Why they are rejected" placeholder="Why, e.g. attach the itemised receipt" value={note} maxLength={500} required
                onChange={(e) => { setNote(e.target.value); }} />
              <button type="submit" className="danger" disabled={note.trim() === "" || review.isPending}>Reject {waiting.length}</button>
              <button type="button" className="link" onClick={() => { setRejecting(false); }}>Cancel</button>
            </form>
          )}
        </div>
      )}

      {list.data && items.length === 0 && <p className="muted">No claims here.</p>}
      {items.length > 0 && (
        <div className="panel table-wrap">
          <table className="claims-table">
            <thead><tr>
              <th><input type="checkbox" aria-label="Select all" checked={chosen.length === items.length}
                onChange={(e) => { setSelected(e.target.checked ? new Set(items.map((c) => c.id)) : new Set()); }} /></th>
              <th>Person</th><th>Receipt</th><th>Type</th><th>Client / candidate</th>
              <th>Purpose</th><th className="num">Amount</th><th>Status</th><th><span className="visually-hidden">Receipt</span></th>
            </tr></thead>
            <tbody>
              {items.map((c) => (
                <tr key={c.id} className={selected.has(c.id) ? "selected" : undefined}>
                  <td><input type="checkbox" aria-label={`Select ${c.userName}'s ${c.merchant ?? c.fileName}`} checked={selected.has(c.id)}
                    onChange={(e) => { setSelected((s) => { const n = new Set(s); if (e.target.checked) n.add(c.id); else n.delete(c.id); return n; }); }} /></td>
                  <td>{c.userName}{c.claimedAt && <span className="sub">claimed {formatDay(c.claimedAt.slice(0, 10))}</span>}</td>
                  <td>{c.receiptDate ? formatDay(c.receiptDate) : "—"}<span className="sub">{c.merchant ?? c.fileName}</span></td>
                  <td>
                    {c.receiptType ? <TypeChip label={fs.label("claim_receipt_type", c.receiptType)} color={fs.color("claim_receipt_type", c.receiptType)} /> : "—"}
                    {c.claimType && <span className="sub">{fs.label("claim_type", c.claimType)}</span>}
                  </td>
                  <td>{linkWords(c.link) || "—"}</td>
                  <td className="claim-purpose-cell">{c.purpose ?? "—"}</td>
                  <td className="num">{c.amount ? formatMoney(c.amount, c.currency) : "—"}</td>
                  <td>
                    <span className={`badge claim-${c.status}`}>{CLAIM_STATUS[c.status]}</span>
                    {c.status === "rejected" && c.reviewNote && <span className="sub claim-why">{c.reviewNote}</span>}
                    <UploadNote upload={c.upload} />
                  </td>
                  <td className="claim-tools"><button type="button" className="link" onClick={() => { setViewing(c); }}>Receipt</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {viewing && <FileViewer title={`${viewing.userName}: ${viewing.fileName}`} path={`/v1/claims/${viewing.id}/receipt`} type={viewing.contentType}
        onClose={() => { setViewing(null); }} />}
    </section>
  );
}
