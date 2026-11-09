// A client's terms of business: their rates (one for any salary, or tiers by
// annual salary), each with the fee, guarantee, payment terms and a remark
// for another payment method; these fill in each new placement for them (the
// rate for its salary), and can still be changed there.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { formatDay, formatWhen } from "../labels";
import { CompanyDocuments } from "./CompanyDocuments";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { CURRENCIES, fieldErrors } from "./forms";
import { emptyRow, guaranteeLabel, rangeLabel, rowFromTier, type TierRow, tierFromRow } from "./termsTiers";

export const companyTermsKey = (id: string) => ["company", id, "terms"] as const;

type Terms = Schemas["CompanyTerms"];

export function CompanyTerms({ companyId, permissions }: { companyId: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const terms = useQuery({
    queryKey: companyTermsKey(companyId),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/terms", { params: { path: { companyId } } })),
  });
  const t = terms.data;
  const canEdit = permissions.includes("companies:update");
  return (
    <section className="panel" aria-label="Terms of business">
      <div className="row spread">
        <div>
          <h3>Terms of business</h3>
          <p className="muted small">Each new placement for this client starts from the rate for its salary; it can be changed on the placement.</p>
        </div>
        {canEdit && t && !editing && <button type="button" onClick={() => { setEditing(true); }}>{t.version === 0 ? "Add terms" : "Edit"}</button>}
      </div>
      <ErrorMessage error={terms.error} />
      {t && (editing ? <TermsForm companyId={companyId} terms={t} onDone={() => { setEditing(false); }} /> : <TermsView t={t} />)}
      <CompanyDocuments companyId={companyId} section="terms" permissions={permissions} />
    </section>
  );
}

function TermsView({ t }: { t: Terms }) {
  if (t.version === 0) return <p className="muted">No terms recorded yet: the fee, guarantee and payment terms they signed, one rate or tiers by salary.</p>;
  return (
    <>
      {t.tiers.length > 0 && (
        <div className="table-scroll">
          <table className="terms-table">
            <thead><tr><th>Annual salary</th><th>Fee</th><th>Guarantee</th><th>Payment terms</th><th>Payment method / remarks</th></tr></thead>
            <tbody>
              {t.tiers.map((r, i) => (
                <tr key={i}>
                  <td>{rangeLabel(r, t.salaryCurrency)}</td>
                  <td className="strong">{r.feePercent ? `${r.feePercent}%` : "—"}</td>
                  <td>{guaranteeLabel(r.guaranteeDays)}</td>
                  <td>{r.paymentTermsDays !== null ? `${String(r.paymentTermsDays)} days` : "—"}</td>
                  <td className="pre">{r.remarks ?? ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="muted small">
        {[t.signedOn && `Signed ${formatDay(t.signedOn)}`, t.updatedAt && `saved ${formatWhen(t.updatedAt)}`].filter(Boolean).join(" · ")}
      </p>
      {t.notes && <p className="pre">{t.notes}</p>}
    </>
  );
}

/** A rate's fields, as its errors name them. */
const ROW_FIELDS = {
  salaryFrom: "salary from", salaryBelow: "below", feePercent: "fee", guaranteeDays: "guarantee (in days)",
  paymentTermsDays: "payment terms", remarks: "payment method / remarks",
};

function TermsForm({ companyId, terms, onDone }: { companyId: string; terms: Terms; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [rows, setRows] = useState<TierRow[]>(() => (terms.tiers.length > 0 ? terms.tiers.map(rowFromTier) : [emptyRow()]));
  const [f, setF] = useState({ currency: terms.salaryCurrency, signedOn: terms.signedOn ?? "", notes: terms.notes ?? "" });
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/companies/{companyId}/terms", {
      params: { path: { companyId } },
      body: { version: terms.version, tiers: rows.map(tierFromRow), salaryCurrency: f.currency, signedOn: f.signedOn || null, notes: f.notes.trim() || null },
    })),
    onSuccess: (saved) => { queryClient.setQueryData(companyTermsKey(companyId), saved); onDone(); },
  });
  const errors = fieldErrors(save.error);
  const setRow = (i: number, k: keyof TierRow) => (e: { target: { value: string } }) => {
    setRows(rows.map((r, j) => (j === i ? { ...r, [k]: e.target.value } : r)));
  };
  const err = (i: number, k: string) => errors[`tiers.${String(i)}.${k}`];
  const rowErrors = (i: number) => Object.entries(ROW_FIELDS).flatMap(([k, label]) => {
    const e = err(i, k);
    return e ? [`${label}: ${e}`] : [];
  });
  return (
    <form aria-label="Terms of business form" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <p className="muted small">
        One row for one rate whatever the salary; or a row per salary band (leave &quot;from&quot; empty for the lowest band and &quot;below&quot; empty for the highest).
      </p>
      <div className="table-scroll">
        <table className="terms-table editing">
          <thead>
            <tr>
              <th>Annual salary from</th><th>Below</th><th>Fee %</th><th>Guarantee</th><th>Payment terms (days)</th><th>Payment method / remarks</th><th />
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => {
              const n = String(i + 1);
              return (
                <tr key={i}>
                  <td><input inputMode="decimal" aria-label={`Rate ${n}: annual salary from`} placeholder="any" value={r.from} onChange={setRow(i, "from")} /></td>
                  <td><input inputMode="decimal" aria-label={`Rate ${n}: below`} placeholder="any" value={r.below} onChange={setRow(i, "below")} /></td>
                  <td><input inputMode="decimal" aria-label={`Rate ${n}: fee %`} placeholder="e.g. 18" value={r.fee} onChange={setRow(i, "fee")} /></td>
                  <td>
                    <div className="guarantee">
                      <input type="number" min={0} aria-label={`Rate ${n}: guarantee`} value={r.guarantee} onChange={setRow(i, "guarantee")} />
                      <select aria-label={`Rate ${n}: guarantee in`} value={r.unit} onChange={setRow(i, "unit")}>
                        <option value="months">months</option><option value="days">days</option>
                      </select>
                    </div>
                  </td>
                  <td><input type="number" min={0} max={365} aria-label={`Rate ${n}: payment terms (days)`} value={r.payment} onChange={setRow(i, "payment")} /></td>
                  <td>
                    <textarea rows={2} maxLength={500} aria-label={`Rate ${n}: payment method / remarks`} placeholder="e.g. 50% on signing, 50% on start"
                      value={r.remarks} onChange={setRow(i, "remarks")} />
                  </td>
                  <td>
                    {rows.length > 1 && (
                      <button type="button" className="link danger" aria-label={`Remove rate ${n}`}
                        onClick={() => { setRows(rows.filter((_, j) => j !== i)); }}>Remove</button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {rows.map((_, i) => rowErrors(i).length > 0 && (
        <p key={i} className="error" role="alert">Rate {i + 1}: {rowErrors(i).join("; ")}</p>
      ))}
      {errors.tiers && <p className="error" role="alert">{errors.tiers}</p>}
      <div className="row">
        <button type="button" onClick={() => { setRows([...rows, emptyRow()]); }} disabled={rows.length >= 20}>+ Add a salary band</button>
      </div>
      <div className="form-grid">
        <Field label="Salary bands in" error={errors.salaryCurrency}>
          <select value={f.currency} onChange={(e) => { setF({ ...f, currency: e.target.value }); }}>
            {CURRENCIES.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </Field>
        <Field label="Signed on" error={errors.signedOn}>
          <input type="date" value={f.signedOn} onChange={(e) => { setF({ ...f, signedOn: e.target.value }); }} />
        </Field>
        <div className="full">
          <Field label="Notes (replacement, exclusivity, anything else)" error={errors.notes}>
            <textarea rows={2} maxLength={2000} value={f.notes} onChange={(e) => { setF({ ...f, notes: e.target.value }); }} />
          </Field>
        </div>
        <div className="row full">
          <button type="submit" disabled={save.isPending}>Save terms</button>
          <button type="button" className="link" onClick={onDone}>Cancel</button>
        </div>
      </div>
      <ErrorMessage error={save.error} />
    </form>
  );
}
