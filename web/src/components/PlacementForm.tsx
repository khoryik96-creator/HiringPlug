import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { placementsKey, usersKey } from "../api/keys";
import { INVOICE_STATUS, keysOf, PAY_PERIOD, PLACEMENT_STATUS, REPLACEMENT, formatMoney } from "../labels";
import { Field } from "./Field";
import { CURRENCIES, fieldErrors } from "./forms";
import { createBody, feePreview, formFromPlacement, invoicingBody, type PlacementFormState, updateBody, withDefaults } from "./placementFormState";
import { InvoicingFields } from "./PlacementInvoicing";
import { pickTier } from "./termsTiers";
import { SaveError } from "./SaveError";

type Placement = Schemas["Placement"];

/**
 * Record a placement for an application (it moves to placed), or change one:
 * start, guarantee, recruiter credited, what happened, the invoice, and (for
 * those allowed) the salary and fee.
 */
export function PlacementForm({ applicationId, placement, permissions, onDone }: {
  applicationId: string; placement?: Placement; permissions: Permission[]; onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const users = useQuery({ queryKey: usersKey, queryFn: async () => unwrap(await api.GET("/v1/users")).items, staleTime: 60_000 });
  const withMoney = permissions.includes("placements.fees:update");
  const [opened] = useState(() => ({ version: placement?.version ?? 0 }));
  // When recording, "" means you (the server's default).
  const [form, setForm] = useState(() => formFromPlacement(placement, ""));
  // A new placement starts from the client's terms of business.
  const defaults = useQuery({
    queryKey: ["placement-defaults", applicationId],
    queryFn: async () => unwrap(await api.GET("/v1/applications/{applicationId}/placement-defaults", { params: { path: { applicationId } } })),
    enabled: !placement,
  });
  const applied = useRef(false);
  useEffect(() => {
    if (defaults.data && !applied.current) {
      applied.current = true;
      setForm((f) => withDefaults(f, defaults.data, withMoney));
    }
  }, [defaults.data, withMoney]);
  // Tiered terms: the rate for the salary typed in fills the fee, guarantee
  // and payment terms when the salary moves into it (and can still be changed).
  const tier = defaults.data ? pickTier(defaults.data.tiers, defaults.data.salaryCurrency,
    { amount: form.salaryAmount, currency: form.salaryCurrency, period: form.salaryPeriod }) : -1;
  const appliedTier = useRef(-1);
  useEffect(() => {
    const rate = defaults.data?.tiers[tier];
    if (placement || !rate || appliedTier.current === tier) return;
    appliedTier.current = tier;
    setForm((f) => ({
      ...f,
      guaranteeDays: rate.guaranteeDays !== null ? String(rate.guaranteeDays) : f.guaranteeDays,
      paymentTermsDays: rate.paymentTermsDays !== null ? String(rate.paymentTermsDays) : f.paymentTermsDays,
      feePercent: withMoney && f.feeKind === "percent" && rate.feePercent ? rate.feePercent : f.feePercent,
    }));
  }, [tier, defaults.data, placement, withMoney]);
  // Recorded but not yet finished (its invoicing failed to save): saving
  // again changes it rather than recording it twice.
  const [recorded, setRecorded] = useState<Placement | null>(null);
  const save = useMutation({
    mutationFn: async () => {
      const current = placement ?? recorded;
      const saved = current
        ? unwrap(await api.PATCH("/v1/placements/{placementId}", {
          params: { path: { placementId: current.id } }, body: updateBody(form, placement ? opened.version : current.version, withMoney),
        }))
        : unwrap(await api.POST("/v1/applications/{applicationId}/placement", {
          params: { path: { applicationId } }, body: createBody(form, withMoney),
        }));
      if (!placement) setRecorded(saved);
      return unwrap(await api.PUT("/v1/placements/{placementId}/invoicing", {
        params: { path: { placementId: saved.id } }, body: invoicingBody(form),
      }));
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: placementsKey });
      await queryClient.invalidateQueries({ queryKey: ["applications"] }); // the stage moves to placed
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = (key: keyof PlacementFormState) => (e: { target: { value: string } }) => {
    setForm((f) => ({ ...f, [key]: e.target.value }));
  };
  const preview = feePreview(form);
  const shown = ["startDate", "guaranteeDays", "recruiterId", "leftOn", "paidOn", "money.feePercent", "money.feeAmount", "contactId", "paymentTermsDays", "dueOn"];

  return (
    <form className="panel form-grid" aria-label={placement ? "Change placement" : "Record placement"}
      onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <Field label="Start date" error={errors.startDate}>
        <input type="date" value={form.startDate} onChange={set("startDate")} />
      </Field>
      <Field label="Guarantee (days)" error={errors.guaranteeDays}>
        <input type="number" min={0} max={365} value={form.guaranteeDays} onChange={set("guaranteeDays")} />
      </Field>
      <Field label="Recruiter credited" error={errors.recruiterId}>
        <select value={form.recruiterId} onChange={set("recruiterId")}>
          <option value="">{placement ? "Nobody" : "You"}</option>
          {users.data?.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </select>
      </Field>
      {placement && (
        <>
          <Field label="What happened">
            <select value={form.status} onChange={set("status")}>
              {keysOf(PLACEMENT_STATUS).map((s) => <option key={s} value={s}>{PLACEMENT_STATUS[s]}</option>)}
            </select>
          </Field>
          {form.status === "fell_through" && (
            <Field label="Left on" error={errors.leftOn}><input type="date" value={form.leftOn} onChange={set("leftOn")} /></Field>
          )}
          {form.status === "fell_through" && placement.status === "fell_through" && (
            <Field label="Replacement">
              <select value={form.replacement} onChange={set("replacement")}>
                {(["owed", "provided", "refunded"] as const).map((r) => <option key={r} value={r}>{REPLACEMENT[r]}</option>)}
              </select>
            </Field>
          )}
          {form.status !== "active" && (
            <Field label="Why"><input value={form.statusNote} onChange={set("statusNote")} /></Field>
          )}
          <Field label="Invoice">
            <select value={form.invoiceStatus} onChange={set("invoiceStatus")}>
              {keysOf(INVOICE_STATUS).map((s) => <option key={s} value={s}>{INVOICE_STATUS[s]}</option>)}
            </select>
          </Field>
          {form.invoiceStatus !== "not_invoiced" && (
            <>
              <Field label="Invoice number"><input value={form.invoiceNumber} onChange={set("invoiceNumber")} /></Field>
              <Field label="Invoiced on"><input type="date" value={form.invoicedOn} onChange={set("invoicedOn")} /></Field>
            </>
          )}
          {form.invoiceStatus === "paid" && (
            <Field label="Paid on" error={errors.paidOn}><input type="date" value={form.paidOn} onChange={set("paidOn")} /></Field>
          )}
        </>
      )}
      {withMoney && (
        <fieldset className="form-grid full">
          <legend>Salary and fee</legend>
          <datalist id="placement-currencies">{CURRENCIES.map((c) => <option key={c} value={c} />)}</datalist>
          <Field label="Agreed salary"><input inputMode="decimal" value={form.salaryAmount} onChange={set("salaryAmount")} /></Field>
          <Field label="Currency"><input list="placement-currencies" className="code" value={form.salaryCurrency} onChange={set("salaryCurrency")} /></Field>
          <Field label="Per">
            <select value={form.salaryPeriod} onChange={set("salaryPeriod")}>
              {keysOf(PAY_PERIOD).map((p) => <option key={p} value={p}>{PAY_PERIOD[p]}</option>)}
            </select>
          </Field>
          <Field label="Fee">
            <select value={form.feeKind} onChange={set("feeKind")}>
              <option value="percent">Percentage of the yearly salary</option>
              <option value="fixed">Fixed amount</option>
            </select>
          </Field>
          {form.feeKind === "percent" ? (
            <Field label="Fee %" error={errors["money.feePercent"]}>
              <input inputMode="decimal" value={form.feePercent} onChange={set("feePercent")} />
            </Field>
          ) : (
            <>
              <Field label="Fee amount" error={errors["money.feeAmount"]}>
                <input inputMode="decimal" value={form.feeAmount} onChange={set("feeAmount")} />
              </Field>
              <Field label="Fee currency"><input list="placement-currencies" className="code" value={form.feeCurrency} onChange={set("feeCurrency")} /></Field>
            </>
          )}
          {preview && <p className="muted full" role="status">Fee: {formatMoney(preview, form.salaryCurrency.toUpperCase())}</p>}
        </fieldset>
      )}
      <InvoicingFields form={form} setForm={setForm} companyId={placement?.companyId ?? defaults.data?.companyId}
        terms={placement ? undefined : defaults.data} tier={tier} errors={errors} />
      <SaveError error={save.error} shown={shown} onReload={onDone} />
      <div className="row full">
        <button type="submit" disabled={save.isPending || !form.startDate}>{placement ? "Save" : "Record placement"}</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}
