// A person's own claims: receipts dropped here are read (OCR) into claims to
// fill in and submit; below them, the claims already sent and what became of them.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../../api/client";
import { uploadClaimReceipt } from "../../api/files";
import { claimsKey } from "../../api/keys";
import { formatDay, formatMoney } from "../../labels";
import { DropZone } from "../DropZone";
import { ErrorMessage } from "../ErrorMessage";
import { FileViewer } from "../FileViewer";
import { useFieldSettings } from "../useFieldSettings";
import { type Claim, CLAIM_STATUS, formatTotals, totals } from "./claims";
import { ClaimRow } from "./ClaimRow";
import { linkWords } from "./LinkPicker";

/** Receipts: a photo or a PDF. */
export const RECEIPT_TYPES = ".pdf,.png,.jpg,.jpeg,.tif,.tiff,.bmp,.webp";

export function MyClaims({ permissions }: { permissions: Permission[] }) {
  const client = useQueryClient();
  const fs = useFieldSettings();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [viewing, setViewing] = useState<Claim | null>(null);
  /** Files being uploaded: "uploading", or why one failed (done ones leave the list). */
  const [uploads, setUploads] = useState<{ name: string; state: string }[]>([]);
  const list = useQuery({
    queryKey: [...claimsKey, "mine"],
    queryFn: async () => unwrap(await api.GET("/v1/claims", { params: { query: { limit: 500 } } })),
    refetchInterval: (q) => (q.state.data?.items.some((c) => c.ocrStatus === "reading") ? 2000 : false),
  });
  const refresh = () => client.invalidateQueries({ queryKey: claimsKey });

  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      setUploads(files.map((f) => ({ name: f.name, state: "uploading" })));
      for (const [i, file] of files.entries()) {
        let state = "done";
        try {
          await uploadClaimReceipt(file, file.name);
        } catch (e) {
          state = e instanceof Error ? e.message : "The upload failed.";
        }
        setUploads((u) => u.map((x, j) => (j === i ? { ...x, state } : x)));
        await refresh();
      }
      setUploads((u) => u.filter((x) => x.state !== "done"));
    },
  });
  const submit = useMutation({
    mutationFn: async (claims: Claim[]) => unwrap(await api.POST("/v1/claims/submit", {
      body: { claims: claims.map((c) => ({ id: c.id, version: c.version })) },
    })),
    onSuccess: async () => { setSelected(new Set()); await refresh(); },
  });
  const remove = useMutation({
    mutationFn: async (c: Claim) => { unwrap(await api.DELETE("/v1/claims/{claimId}", { params: { path: { claimId: c.id } } })); },
    onSuccess: refresh,
  });
  const withdraw = useMutation({
    mutationFn: async (c: Claim) => unwrap(await api.POST("/v1/claims/{claimId}/withdraw", {
      params: { path: { claimId: c.id } }, body: { version: c.version },
    })),
    onSuccess: refresh,
  });

  const items = list.data?.items ?? [];
  const open = items.filter((c) => c.status === "draft" || c.status === "rejected");
  const sent = items.filter((c) => c.status !== "draft" && c.status !== "rejected");
  const ready = open.filter((c) => c.missing.length === 0 && c.ocrStatus !== "reading");
  const chosen = ready.filter((c) => selected.has(c.id));
  const choose = (id: string, on: boolean) => {
    setSelected((s) => { const n = new Set(s); if (on) n.add(id); else n.delete(id); return n; });
  };

  return (
    <section className="my-claims" aria-label="My claims">
      <DropZone accept={RECEIPT_TYPES} multiple disabled={upload.isPending} label="Drop receipts here"
        hint="photos or PDFs; each is read and becomes a claim to check" onFiles={(files) => { upload.mutate(files); }} />
      {uploads.length > 0 && (
        <ul className="claim-uploads">
          {uploads.map((u, i) => (
            <li key={`${String(i)}-${u.name}`}>{u.name}: {u.state === "uploading" || u.state === "done"
              ? <span className="muted">{u.state === "done" ? "uploaded" : "uploading…"}</span> : <span className="error">{u.state}</span>}</li>
          ))}
        </ul>
      )}
      <ErrorMessage error={list.error} />

      <div className="row spread claim-submit-bar">
        <h3>To submit {open.length > 0 && <span className="muted">({open.length})</span>}</h3>
        {open.length > 0 && (
          <div className="row">
            <button type="button" className="link" disabled={ready.length === 0}
              onClick={() => { setSelected(new Set(ready.map((c) => c.id))); }}>Select all ready ({ready.length})</button>
            <button type="button" className="primary" disabled={chosen.length === 0 || submit.isPending}
              onClick={() => { submit.mutate(chosen); }}>
              {submit.isPending ? "Submitting…"
                : chosen.length === 0 ? "Submit selected"
                  : `Submit ${String(chosen.length)} claim${chosen.length === 1 ? "" : "s"} · ${formatTotals(totals(chosen))}`}
            </button>
          </div>
        )}
      </div>
      <ErrorMessage error={submit.error ?? remove.error ?? withdraw.error} />
      {list.data && open.length === 0 && <p className="muted">No receipts waiting. Drop one above to start a claim.</p>}
      {open.length > 0 && <p className="small muted claim-legend"><span className="from-receipt-key" aria-hidden="true" /> Striped values were read from the receipt: check them.</p>}
      <div className="claim-rows">
        {open.map((c) => (
          <ClaimRow key={c.id} claim={c} permissions={permissions} selected={selected.has(c.id)} onSelect={(on) => { choose(c.id, on); }}
            onView={() => { setViewing(c); }}
            onDelete={() => { if (window.confirm(`Delete the claim for ${c.fileName}?`)) remove.mutate(c); }} />
        ))}
      </div>

      <h3>Sent</h3>
      {list.data && sent.length === 0 && <p className="muted">Nothing sent yet.</p>}
      {sent.length > 0 && (
        <div className="panel table-wrap">
          <table className="claims-table">
            <thead><tr>
              <th>Receipt</th><th>Type</th><th>Client / candidate</th><th className="num">Amount</th><th>Claimed on</th><th>Status</th>
              <th><span className="visually-hidden">Actions</span></th>
            </tr></thead>
            <tbody>
              {sent.map((c) => (
                <tr key={c.id}>
                  <td>{c.receiptDate ? formatDay(c.receiptDate) : "—"}<span className="sub">{c.merchant ?? c.fileName}</span></td>
                  <td>
                    {c.receiptType ? <TypeChip label={fs.label("claim_receipt_type", c.receiptType)} color={fs.color("claim_receipt_type", c.receiptType)} /> : "—"}
                    {c.claimType && <span className="sub">{fs.label("claim_type", c.claimType)}</span>}
                  </td>
                  <td>{linkWords(c.link) || "—"}</td>
                  <td className="num">{c.amount ? formatMoney(c.amount, c.currency) : "—"}</td>
                  <td>{c.claimedAt ? formatDay(c.claimedAt.slice(0, 10)) : "—"}</td>
                  <td>
                    <span className={`badge claim-${c.status}`}>{CLAIM_STATUS[c.status]}</span>
                    {c.paidAt && <span className="sub">on {formatDay(c.paidAt.slice(0, 10))}</span>}
                  </td>
                  <td className="claim-tools">
                    <button type="button" className="link" onClick={() => { setViewing(c); }}>Receipt</button>
                    {c.status === "submitted" && (
                      <button type="button" className="link" disabled={withdraw.isPending} onClick={() => { withdraw.mutate(c); }}>Withdraw</button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {viewing && <FileViewer title={viewing.fileName} path={`/v1/claims/${viewing.id}/receipt`} type={viewing.contentType}
        onClose={() => { setViewing(null); }} />}
    </section>
  );
}

/** A receipt type with its colour beside it (the word carries it; the swatch helps the eye). */
export function TypeChip({ label, color }: { label: string; color: string }) {
  return <span className="claim-type-chip"><span className="swatch" style={{ background: color }} aria-hidden="true" />{label}</span>;
}
