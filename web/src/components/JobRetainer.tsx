// A job's retainer (a retained search): the stages the client pays while the
// search runs, taken off the fee of the first placement. Fees only.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { placementsKey } from "../api/keys";
import { INVOICE_STATUS, formatDay, formatMoney, keysOf } from "../labels";
import { CURRENCIES, fieldErrors, FormInvalid } from "./forms";
import { todayISO } from "./placementTimeline";
import { blankStage, DUE_KIND, dueText, retainerBody, retainerForm, retainerProblems, type RetainerForm, type StageRow } from "./retainerFormState";
import { SaveError } from "./SaveError";
import { ErrorMessage } from "./ErrorMessage";

const retainerKey = (jobId: string) => ["retainer", jobId];
type Retainer = Schemas["JobRetainer"];

export function JobRetainer({ jobId, permissions }: { jobId: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const canSee = permissions.includes("placements.fees:read");
  const canEdit = permissions.includes("placements.fees:update");
  const q = useQuery({
    queryKey: retainerKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/retainer", { params: { path: { jobId } } })),
    enabled: canSee,
  });
  if (!canSee) return null;
  const r = q.data;
  return (
    <section className="panel" aria-label="Retainer">
      <div className="row spread">
        <h3>Retainer</h3>
        {canEdit && r && !editing && <button type="button" onClick={() => { setEditing(true); }}>{r.version === 0 ? "Add retainer" : "Edit"}</button>}
      </div>
      <ErrorMessage error={q.error} />
      {r && (editing
        ? <RetainerEditor jobId={jobId} retainer={r} onDone={() => { setEditing(false); }} />
        : <RetainerTable r={r} />)}
    </section>
  );
}

function RetainerTable({ r }: { r: Retainer }) {
  if (r.version === 0) return <p className="muted">None. A retained search is paid in stages, taken off the placement fee.</p>;
  const cur = r.currency ?? "";
  const today = todayISO();
  return (
    <div className="table-wrap">
      <table className="retainer-table">
        <thead><tr><th>Stage</th><th className="num">Amount</th><th>Due</th><th>Invoice</th></tr></thead>
        <tbody>
          {r.stages.map((s, i) => {
            const late = s.invoiceStatus !== "paid" && s.dueOn !== null && s.dueOn < today;
            const pill = s.invoiceStatus === "paid" ? "paid" : late ? "overdue" : s.invoiceStatus === "invoiced" ? "due" : "none";
            return (
              <tr key={i}>
                <td>{s.label}</td>
                <td className="num">{formatMoney(s.amount, cur)}</td>
                <td>{dueText(s, formatDay)}</td>
                <td><span className={`pill ${pill}`}>{late ? "Overdue" : INVOICE_STATUS[s.invoiceStatus]}</span>{s.invoiceNumber && <span className="muted small"> {s.invoiceNumber}</span>}</td>
              </tr>
            );
          })}
        </tbody>
        <tfoot><tr><td>Total</td><td className="num"><strong>{formatMoney(r.total, cur)}</strong></td><td colSpan={2} className="muted small">
          {r.signedOn && <>Signed {formatDay(r.signedOn)} · </>}{r.placementId ? "taken off the placement fee" : "taken off the fee once placed"}
        </td></tr></tfoot>
      </table>
    </div>
  );
}

function RetainerEditor({ jobId, retainer, onDone }: { jobId: string; retainer: Retainer; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<RetainerForm>(() => retainerForm(retainer));
  const save = useMutation({
    mutationFn: async () => {
      const body = retainerBody(form, retainer.version);
      const problems = retainerProblems(body);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      return unwrap(await api.PUT("/v1/jobs/{jobId}/retainer", { params: { path: { jobId } }, body }));
    },
    onSuccess: async (saved) => {
      queryClient.setQueryData(retainerKey(jobId), saved);
      await queryClient.invalidateQueries({ queryKey: placementsKey });
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const setStage = (i: number, patch: Partial<StageRow>) => {
    setForm((f) => ({ ...f, stages: f.stages.map((s, j) => (j === i ? { ...s, ...patch } : s)) }));
  };
  const err = (i: number, field: string) => errors[`stages.${String(i)}.${field}`];
  return (
    <form className="retainer-form" aria-label="Edit retainer" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <div className="row">
        <label>Currency <input className="code" list="retainer-currencies" size={4} value={form.currency}
          onChange={(e) => { setForm({ ...form, currency: e.target.value }); }} /></label>
        <datalist id="retainer-currencies">{CURRENCIES.map((c) => <option key={c} value={c} />)}</datalist>
        <label>Signed on <input type="date" value={form.signedOn} onChange={(e) => { setForm({ ...form, signedOn: e.target.value }); }} /></label>
        {errors.currency && <span className="error small">{errors.currency}</span>}
      </div>
      <div className="table-wrap">
        <table className="retainer-table edit">
          <thead><tr><th>Stage</th><th>Amount</th><th>Due</th><th>Invoice</th><th><span className="visually-hidden">Remove</span></th></tr></thead>
          <tbody>
            {form.stages.map((s, i) => (
              <tr key={i}>
                <td>
                  <input aria-label={`Stage ${String(i + 1)} name`} value={s.label} maxLength={100} onChange={(e) => { setStage(i, { label: e.target.value }); }} />
                  {err(i, "label") && <div className="error small">{err(i, "label")}</div>}
                </td>
                <td>
                  <input aria-label={`Stage ${String(i + 1)} amount`} inputMode="decimal" className="num" value={s.amount} onChange={(e) => { setStage(i, { amount: e.target.value }); }} />
                  {err(i, "amount") && <div className="error small">{err(i, "amount")}</div>}
                </td>
                <td>
                  <div className="row tight">
                    <select aria-label={`Stage ${String(i + 1)} due`} value={s.dueKind} onChange={(e) => { setStage(i, { dueKind: e.target.value as StageRow["dueKind"] }); }}>
                      {keysOf(DUE_KIND).map((k) => <option key={k} value={k}>{DUE_KIND[k]}</option>)}
                    </select>
                    {s.dueKind === "on_date"
                      ? <input type="date" aria-label={`Stage ${String(i + 1)} due on`} value={s.dueOn} onChange={(e) => { setStage(i, { dueOn: e.target.value }); }} />
                      : <input type="number" min={0} max={730} className="days" aria-label={`Stage ${String(i + 1)} days`} value={s.dueDays} onChange={(e) => { setStage(i, { dueDays: e.target.value }); }} />}
                  </div>
                  {(err(i, "dueOn") ?? err(i, "dueDays")) && <div className="error small">{err(i, "dueOn") ?? err(i, "dueDays")}</div>}
                </td>
                <td>
                  <div className="row tight">
                    <select aria-label={`Stage ${String(i + 1)} invoice`} value={s.invoiceStatus} onChange={(e) => { setStage(i, { invoiceStatus: e.target.value as StageRow["invoiceStatus"] }); }}>
                      {keysOf(INVOICE_STATUS).map((k) => <option key={k} value={k}>{INVOICE_STATUS[k]}</option>)}
                    </select>
                    {s.invoiceStatus !== "not_invoiced" && (
                      <input aria-label={`Stage ${String(i + 1)} invoice number`} placeholder="No." className="inv-no" value={s.invoiceNumber} onChange={(e) => { setStage(i, { invoiceNumber: e.target.value }); }} />
                    )}
                    {s.invoiceStatus === "paid" && (
                      <input type="date" aria-label={`Stage ${String(i + 1)} paid on`} value={s.paidOn} onChange={(e) => { setStage(i, { paidOn: e.target.value }); }} />
                    )}
                  </div>
                  {err(i, "paidOn") && <div className="error small">{err(i, "paidOn")}</div>}
                </td>
                <td><button type="button" className="link" aria-label={`Remove stage ${String(i + 1)}`}
                  onClick={() => { setForm((f) => ({ ...f, stages: f.stages.filter((_, j) => j !== i) })); }}>✕</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {form.stages.length < 12 && (
        <button type="button" className="link" onClick={() => { setForm((f) => ({ ...f, stages: [...f.stages, blankStage()] })); }}>+ Add stage</button>
      )}
      <SaveError error={save.error} shown={["currency", ...form.stages.flatMap((_, i) => ["label", "amount", "dueOn", "dueDays", "paidOn"].map((f) => `stages.${String(i)}.${f}`))]} onReload={onDone} />
      <div className="row">
        <button type="submit" disabled={save.isPending}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}
