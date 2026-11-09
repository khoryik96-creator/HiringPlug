// A client's terms of business: the fee, payment terms and guarantee they
// signed, which fill in each new placement for them (and can still be
// changed there).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { formatDay, formatWhen } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { fieldErrors } from "./forms";

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
          <p className="muted small">Each new placement for this client starts from these; they can be changed on the placement.</p>
        </div>
        {canEdit && t && !editing && <button type="button" onClick={() => { setEditing(true); }}>{t.version === 0 ? "Add terms" : "Edit"}</button>}
      </div>
      <ErrorMessage error={terms.error} />
      {t && (editing ? <TermsForm companyId={companyId} terms={t} onDone={() => { setEditing(false); }} /> : <TermsView t={t} />)}
    </section>
  );
}

function TermsView({ t }: { t: Terms }) {
  if (t.version === 0) return <p className="muted">No terms recorded yet: the fee, payment terms and guarantee they signed.</p>;
  const tile = (label: string, value: string | null) => (
    <div className="term"><span className="term-label">{label}</span><span className="term-value">{value ?? "—"}</span></div>
  );
  return (
    <>
      <div className="terms">
        {tile("Fee", t.feePercent ? `${t.feePercent}% of annual salary` : null)}
        {tile("Payment terms", t.paymentTermsDays !== null ? `${String(t.paymentTermsDays)} days` : null)}
        {tile("Guarantee", t.guaranteeDays !== null ? `${String(t.guaranteeDays)} days` : null)}
        {tile("Signed", t.signedOn ? formatDay(t.signedOn) : null)}
      </div>
      {t.notes && <p className="pre">{t.notes}</p>}
      {t.updatedAt && <p className="muted small">Saved {formatWhen(t.updatedAt)}</p>}
    </>
  );
}

function TermsForm({ companyId, terms, onDone }: { companyId: string; terms: Terms; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [f, setF] = useState({
    feePercent: terms.feePercent ?? "", paymentTermsDays: terms.paymentTermsDays?.toString() ?? "30",
    guaranteeDays: terms.guaranteeDays?.toString() ?? "90", signedOn: terms.signedOn ?? "", notes: terms.notes ?? "",
  });
  const num = (s: string) => (s.trim() === "" ? null : Number(s));
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/companies/{companyId}/terms", {
      params: { path: { companyId } },
      body: {
        version: terms.version, feePercent: f.feePercent.trim().replace(/%$/, "") || null, paymentTermsDays: num(f.paymentTermsDays),
        guaranteeDays: num(f.guaranteeDays), signedOn: f.signedOn || null, notes: f.notes.trim() || null,
      },
    })),
    onSuccess: (saved) => { queryClient.setQueryData(companyTermsKey(companyId), saved); onDone(); },
  });
  const errors = fieldErrors(save.error);
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => { setF({ ...f, [k]: e.target.value }); };
  return (
    <form className="form-grid" aria-label="Terms of business form" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <Field label="Fee (% of annual salary)" error={errors.feePercent}>
        <input inputMode="decimal" placeholder="e.g. 18" value={f.feePercent} onChange={set("feePercent")} />
      </Field>
      <Field label="Payment terms (days)" error={errors.paymentTermsDays}>
        <input type="number" min={0} max={365} value={f.paymentTermsDays} onChange={set("paymentTermsDays")} />
      </Field>
      <Field label="Guarantee (days)" error={errors.guaranteeDays}>
        <input type="number" min={0} max={365} value={f.guaranteeDays} onChange={set("guaranteeDays")} />
      </Field>
      <Field label="Signed on" error={errors.signedOn}>
        <input type="date" value={f.signedOn} onChange={set("signedOn")} />
      </Field>
      <div className="full">
        <Field label="Notes (replacement, exclusivity, anything else)" error={errors.notes}>
          <textarea rows={2} maxLength={2000} value={f.notes} onChange={set("notes")} />
        </Field>
      </div>
      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save terms</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <ErrorMessage error={save.error} />
    </form>
  );
}
