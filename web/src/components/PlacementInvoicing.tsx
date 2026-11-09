// A placement's invoicing: in the form, who receives the invoice (one of the
// client's contacts), the payment terms and due date, and whether it is
// emailed on the start date; on the placement, where the invoice stands,
// with the invoice to download and "Send invoice now".
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { placementsKey } from "../api/keys";
import { formatDay, formatWhen } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { autoDueOn, type PlacementFormState } from "./placementFormState";

type Placement = Schemas["Placement"];

/** The client's contacts, to choose who receives invoices. */
export function useClientContacts(companyId: string | undefined) {
  return useQuery({
    queryKey: ["contacts", "of-company", companyId],
    queryFn: async () => unwrap(await api.GET("/v1/contacts", { params: { query: { companyId: companyId ?? "", limit: 100 } } })).items,
    enabled: !!companyId,
    staleTime: 30_000,
  });
}

/** The form's invoicing fields. */
export function InvoicingFields({ form, setForm, companyId, terms, errors }: {
  form: PlacementFormState; setForm: (f: PlacementFormState) => void; companyId: string | undefined;
  terms?: Schemas["PlacementDefaults"]; errors: Record<string, string>;
}) {
  const contacts = useClientContacts(companyId);
  const auto = autoDueOn(form.startDate, form.paymentTermsDays);
  const chosen = contacts.data?.find((c) => c.id === form.contactId);
  return (
    <fieldset className="form-grid full invoicing">
      <legend>Invoicing</legend>
      {terms && (
        <p className="muted small full">
          {terms.termsSet
            ? <>From {terms.companyName}&apos;s terms of business: {[terms.feePercent && `${terms.feePercent}% fee`, `${String(terms.paymentTermsDays)} days to pay`,
              `${String(terms.guaranteeDays)}-day guarantee`].filter(Boolean).join(" · ")}. Change anything below for this placement.</>
            : <>No terms of business for {terms.companyName} yet: add them on <a href={`#/companies/${terms.companyId}`}>its page</a> to fill these in.</>}
        </p>
      )}
      <Field label="Send the invoice to" error={errors.contactId}>
        <select value={form.contactId} onChange={(e) => { setForm({ ...form, contactId: e.target.value, autoSend: e.target.value ? form.autoSend : false }); }}>
          <option value="">Choose a contact…</option>
          {contacts.data?.map((c) => (
            <option key={c.id} value={c.id} disabled={!c.email}>{c.fullName} — {c.email ?? "no email address"}</option>
          ))}
        </select>
      </Field>
      <Field label="Payment terms (days)" error={errors.paymentTermsDays}>
        <input type="number" min={0} max={365} value={form.paymentTermsDays} onChange={(e) => { setForm({ ...form, paymentTermsDays: e.target.value }); }} />
      </Field>
      <div>
        <Field label="Payment due" error={errors.dueOn}>
          <input type="date" value={form.dueOn || auto} onChange={(e) => { setForm({ ...form, dueOn: e.target.value === auto ? "" : e.target.value }); }} />
        </Field>
        <small className="muted">
          {form.dueOn
            ? <>Set by hand. <button type="button" className="link" onClick={() => { setForm({ ...form, dueOn: "" }); }}>Use start date + terms</button></>
            : "The start date plus the terms."}
        </small>
      </div>
      <label className="check full">
        <input type="checkbox" checked={form.autoSend} disabled={!chosen?.email}
          onChange={(e) => { setForm({ ...form, autoSend: e.target.checked }); }} />{" "}
        Email the invoice to {chosen ? chosen.fullName : "them"} on the start date{form.startDate ? ` (${formatDay(form.startDate)})` : ""}, from your
        {" "}<a href="#/invoicing">invoice template</a>
      </label>
    </fieldset>
  );
}

const TYPES = { pdf: "application/pdf", docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document" } as const;

/** Where a placement's invoice stands, and what can be done with it. */
export function PlacementInvoice({ p, permissions }: { p: Placement; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<unknown>(null);
  const inv = p.invoicing;
  const seeFees = permissions.includes("placements.fees:read");
  const send = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/placements/{placementId}/invoice", { params: { path: { placementId: p.id } } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: placementsKey }),
  });
  const get = (format: keyof typeof TYPES) => {
    setError(null);
    downloadFrom(`/v1/placements/${p.id}/invoice/file?format=${format}`, `Invoice ${p.invoiceNumber ?? p.candidateName}.${format}`, TYPES[format])
      .catch((e: unknown) => { setError(e); });
  };
  const to = inv.contactName ? `${inv.contactName}${inv.contactEmail ? ` (${inv.contactEmail})` : ""}` : null;
  let status: string;
  if (inv.sentAt) status = `Invoice ${p.invoiceNumber ?? ""} emailed to ${to ?? "the client"} on ${formatWhen(inv.sentAt)}`;
  else if (p.invoiceStatus !== "not_invoiced") status = `Invoiced${p.invoiceNumber ? ` (${p.invoiceNumber})` : ""}`;
  else if (inv.sendsAt) status = `Emails to ${to ?? "the client"} on ${formatWhen(inv.sendsAt)}`;
  else status = to ? `To ${to}; not emailed automatically` : "No invoice recipient yet";
  const canSend = permissions.includes("placements:manage") && seeFees && p.invoiceStatus === "not_invoiced" && !inv.sentAt && p.status !== "cancelled";
  return (
    <div className="placement-invoice">
      <span className="muted small">{status} · due {formatDay(inv.dueOn)}</span>
      {inv.sendError && !inv.sentAt && <span className="warning small" role="alert"> Not emailed: {inv.sendError}</span>}
      {seeFees && (
        <span className="row">
          <button type="button" className="link" onClick={() => { get("pdf"); }}>{p.invoiceStatus === "not_invoiced" ? "Preview invoice" : "Invoice PDF"}</button>
          <button type="button" className="link" onClick={() => { get("docx"); }}>Word</button>
          {canSend && (
            <button type="button" className="link" disabled={send.isPending || !inv.contactEmail}
              onClick={() => { if (window.confirm(`Email the invoice to ${to ?? "the client"} now?`)) send.mutate(); }}>Send invoice now</button>
          )}
        </span>
      )}
      <ErrorMessage error={error ?? send.error} />
    </div>
  );
}
