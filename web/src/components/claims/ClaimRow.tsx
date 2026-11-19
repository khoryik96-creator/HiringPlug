// One draft or rejected claim, as its claimant fills it in: the receipt, and
// the fields read from it (striped until changed), each saved as it is left.
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../../api/client";
import { claimsKey } from "../../api/keys";
import { ErrorMessage } from "../ErrorMessage";
import { useFieldSettings } from "../useFieldSettings";
import { type Claim, CURRENCIES, FIELD_LABEL } from "./claims";
import { LinkPicker } from "./LinkPicker";

type Update = Omit<Schemas["ClaimUpdate"], "version">;

/** Saves changes to one claim in turn, each with the version the last one returned. */
function useClaimSaver(claim: Claim) {
  const client = useQueryClient();
  const chain = useRef<Promise<void>>(Promise.resolve());
  const version = useRef(claim.version);
  const [pending, setPending] = useState(0);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { version.current = Math.max(version.current, claim.version); }, [claim.version]);
  const save = (change: Update) => {
    setPending((n) => n + 1);
    chain.current = chain.current.then(async () => {
      try {
        const saved = unwrap(await api.PATCH("/v1/claims/{claimId}", {
          params: { path: { claimId: claim.id } }, body: { ...change, version: version.current },
        }));
        version.current = saved.version;
        setError(null);
        client.setQueriesData<Schemas["ClaimPage"]>({ queryKey: [...claimsKey, "mine"] },
          (page) => page && { ...page, items: page.items.map((c) => (c.id === saved.id ? saved : c)) });
      } catch (e) {
        setError(e);
      } finally {
        setPending((n) => n - 1);
      }
    });
  };
  return { save, saving: pending > 0, error };
}

export function ClaimRow({ claim, permissions, selected, onSelect, onView, onDelete }: {
  claim: Claim;
  permissions: Permission[];
  selected: boolean;
  onSelect: (on: boolean) => void;
  onView: () => void;
  onDelete: () => void;
}) {
  const fs = useFieldSettings();
  const { save, saving, error } = useClaimSaver(claim);
  const reading = claim.ocrStatus === "reading";
  const read = (f: Schemas["ClaimField"]) => claim.fromReceipt.includes(f);
  const missing = (f: Schemas["ClaimField"]) => !reading && claim.missing.includes(f);
  /** Props marking a field read from the receipt (striped) or still to fill in (red once reading is done). */
  const mark = (f: Schemas["ClaimField"]) => ({
    className: read(f) ? "from-receipt" : undefined,
    required: true,
    ...(missing(f) ? { "data-missing": "" } : {}),
  });
  const text = (v: string) => (v.trim() === "" ? null : v.trim());
  const ready = claim.missing.length === 0 && !reading;
  const id = (f: string) => `${claim.id}-${f}`;

  return (
    <article className={`claim-row${claim.status === "rejected" ? " rejected" : ""}`} aria-label={`Claim ${claim.fileName}`}>
      <div className="claim-row-head">
        <label className="claim-select">
          <input type="checkbox" checked={selected && ready} disabled={!ready} onChange={(e) => { onSelect(e.target.checked); }}
            aria-label={`Select ${claim.fileName} to submit`} />
        </label>
        <button type="button" className="link claim-file" onClick={onView} title="View the receipt">{claim.fileName}</button>
        {reading && <span className="claim-reading" role="status">Reading the receipt…</span>}
        {claim.ocrStatus === "failed" && <span className="muted small">{claim.ocrError ?? "The receipt could not be read: fill it in."}</span>}
        {claim.status === "rejected" && <span className="badge claim-rejected">Rejected</span>}
        <span className="claim-row-tools">
          {saving && <span className="muted small" role="status">Saving…</span>}
          <button type="button" className="link danger" onClick={onDelete} aria-label={`Delete ${claim.fileName}`}>Delete</button>
        </span>
      </div>
      {claim.status === "rejected" && claim.reviewNote && (
        <p className="claim-note" role="note"><strong>Why it was rejected:</strong> {claim.reviewNote}{claim.reviewedByName ? ` (${claim.reviewedByName})` : ""}</p>
      )}
      <div className="claim-fields">
        <div className="field">
          <label htmlFor={id("date")}>Receipt date</label>
          <input id={id("date")} key={`d${claim.receiptDate ?? ""}`} type="date" defaultValue={claim.receiptDate ?? ""} {...mark("receiptDate")}
            max={new Date().toISOString().slice(0, 10)}
            onBlur={(e) => { if (e.target.value !== (claim.receiptDate ?? "")) save({ receiptDate: e.target.value || null }); }} />
        </div>
        <div className="field">
          <label htmlFor={id("merchant")}>Merchant</label>
          <input id={id("merchant")} key={`m${claim.merchant ?? ""}`} defaultValue={claim.merchant ?? ""} maxLength={200}
            className={read("merchant") ? "from-receipt" : undefined}
            onBlur={(e) => { if (text(e.target.value) !== (claim.merchant ?? null)) save({ merchant: text(e.target.value) }); }} />
        </div>
        <div className="field">
          <label htmlFor={id("rtype")}>Receipt type</label>
          <select id={id("rtype")} value={claim.receiptType ?? ""} {...mark("receiptType")}
            onChange={(e) => { save({ receiptType: e.target.value || null }); }}>
            <option value="">Choose…</option>
            {fs.options("claim_receipt_type", claim.receiptType).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
          </select>
        </div>
        <div className="field claim-amount">
          <label htmlFor={id("amount")}>Amount</label>
          <span className="row">
            <select aria-label="Currency" value={claim.currency} className={read("currency") ? "from-receipt" : undefined}
              onChange={(e) => { save({ currency: e.target.value }); }}>
              {[...new Set([claim.currency, ...CURRENCIES])].map((c) => <option key={c}>{c}</option>)}
            </select>
            <input id={id("amount")} key={`a${claim.amount ?? ""}`} inputMode="decimal" defaultValue={claim.amount ?? ""} {...mark("amount")}
              pattern="[0-9]+(\.[0-9]{1,2})?" placeholder="0.00"
              onBlur={(e) => {
                const v = e.target.value.replaceAll(",", "").trim();
                if (v !== (claim.amount ?? "") && (v === "" || /^\d+(\.\d{1,2})?$/.test(v))) save({ amount: v || null });
              }} />
          </span>
        </div>
        <div className="field">
          <label htmlFor={id("ctype")}>Claim type</label>
          <select id={id("ctype")} value={claim.claimType ?? ""} {...mark("claimType")}
            onChange={(e) => { save({ claimType: e.target.value || null }); }}>
            <option value="">Choose…</option>
            {fs.options("claim_type", claim.claimType).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
          </select>
        </div>
        <div className="field claim-link">
          <label htmlFor={id("link")}>Client / candidate</label>
          <LinkPicker id={id("link")} permissions={permissions}
            value={claim.link ? { kind: claim.link.kind, id: claim.link.id, name: claim.link.name ?? "Not visible to you" } : null}
            onChange={(next) => { save({ link: next ? { kind: next.kind, id: next.id } : null }); }} />
        </div>
        <div className="field claim-purpose">
          <label htmlFor={id("purpose")}>Purpose</label>
          <input id={id("purpose")} key={`p${claim.purpose ?? ""}`} defaultValue={claim.purpose ?? ""} maxLength={500} {...mark("purpose")}
            placeholder="e.g. Lunch with Acme's HR to discuss the QA role"
            onBlur={(e) => { if (text(e.target.value) !== (claim.purpose ?? null)) save({ purpose: text(e.target.value) }); }} />
        </div>
      </div>
      {!reading && claim.missing.length > 0 && (
        <p className="small muted">To submit, fill in the {claim.missing.map((f) => FIELD_LABEL[f]).join(", ")}.</p>
      )}
      <ErrorMessage error={error} />
    </article>
  );
}
