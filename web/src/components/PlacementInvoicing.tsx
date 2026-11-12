// A placement's invoicing: in the form, who receives the invoice (one of the
// client's contacts), the payment terms and due date, and whether it is
// emailed on the start date; on the placement, where the invoice stands,
// with the invoice to download and "Send invoice now".
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { placementsKey } from "../api/keys";
import { formatDay } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { FileViewer } from "./FileViewer";
import { Field } from "./Field";
import { Menu } from "./Menu";
import { autoDueOn, type PlacementFormState } from "./placementFormState";
import { bySalary, guaranteeLabel, rangeLabel } from "./termsTiers";

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
export function InvoicingFields({ form, setForm, companyId, terms, tier = -1, errors }: {
  form: PlacementFormState; setForm: (f: PlacementFormState) => void; companyId: string | undefined;
  terms?: Schemas["PlacementDefaults"]; tier?: number; errors: Record<string, string>;
}) {
  const contacts = useClientContacts(companyId);
  const auto = autoDueOn(form.startDate, form.paymentTermsDays);
  const chosen = contacts.data?.find((c) => c.id === form.contactId);
  return (
    <fieldset className="form-grid full invoicing">
      <legend>Invoicing</legend>
      {terms && <TermsNote terms={terms} tier={tier} />}
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

/** Which of the client's rates filled in the form. */
function TermsNote({ terms, tier }: { terms: Schemas["PlacementDefaults"]; tier: number }) {
  if (!terms.termsSet || terms.tiers.length === 0) {
    return <p className="muted small full">No terms of business for {terms.companyName} yet: add them on <a href={`#/companies/${terms.companyId}`}>its page</a> to fill these in.</p>;
  }
  const rate = terms.tiers[tier];
  const says = rate && [rate.feePercent && `${rate.feePercent}% fee`, rate.paymentTermsDays !== null && `${String(rate.paymentTermsDays)} days to pay`,
    rate.guaranteeDays !== null && `${guaranteeLabel(rate.guaranteeDays)} guarantee`].filter(Boolean).join(" · ");
  return (
    <p className="muted small full" role="note">
      {rate
        ? <>From {terms.companyName}&apos;s terms of business{bySalary(terms.tiers) ? ` (${rangeLabel(rate, terms.salaryCurrency)})` : ""}: {says}.
          {rate.remarks && <> Payment: {rate.remarks}.</>} Change anything below for this placement.</>
        : <>{terms.companyName}&apos;s rates depend on the annual salary ({terms.tiers.map((t) => `${rangeLabel(t, terms.salaryCurrency)}: ${t.feePercent ?? "?"}%`).join("; ")}):
          enter the salary{terms.salaryCurrency !== "MYR" ? ` in ${terms.salaryCurrency}` : ""} to fill them in.</>}
    </p>
  );
}

const TYPES = { pdf: "application/pdf", docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document" } as const;

/** A day from a timestamp, in the user's time zone ("1 Dec 2026"). */
const dayOf = (iso: string) => new Date(iso).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });

/**
 * Where a placement's invoice stands, and what can be done with it: on the
 * placement, a line, an Invoice menu (preview, PDF, Word) and Send now; in a
 * list row ("menu"), one menu with those and any extra items first.
 */
export function PlacementInvoice({ p, permissions, variant = "full", extra }: {
  p: Placement; permissions: Permission[]; variant?: "full" | "menu";
  /** More menu items, before the invoice's (given close(), to call when one is chosen). */
  extra?: (close: () => void) => React.ReactNode;
}) {
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
  const [viewing, setViewing] = useState(false);
  const to = inv.contactName ?? null;
  let status: string;
  if (inv.sentAt) status = `Emailed to ${to ?? "the client"} on ${dayOf(inv.sentAt)}`;
  else if (p.invoiceStatus !== "not_invoiced") status = `Invoiced${p.invoiceNumber ? ` · ${p.invoiceNumber}` : ""}`;
  else if (inv.sendsAt) status = `Emails to ${to ?? "the client"} on ${dayOf(inv.sendsAt)}`;
  else status = to ? `To ${to} · not emailed automatically` : "No invoice recipient yet";
  const canSend = permissions.includes("placements:manage") && seeFees && p.invoiceStatus === "not_invoiced" && !inv.sentAt && p.status !== "cancelled";
  const sendNow = () => {
    if (window.confirm(`Email the invoice to ${to ?? "the client"}${inv.contactEmail ? ` (${inv.contactEmail})` : ""} now?`)) { send.mutate(); setViewing(false); }
  };
  const sendButton = (className: string) => (
    <button type="button" className={className} disabled={send.isPending || !inv.contactEmail} onClick={sendNow}>Send now</button>
  );
  const items = (close: () => void) => {
    const pick = (f: () => void) => () => { close(); f(); };
    return seeFees && (
      <>
        <button type="button" onClick={pick(() => { setViewing(true); })}>{p.invoiceStatus === "not_invoiced" ? "Preview invoice" : "View invoice"}</button>
        <button type="button" onClick={pick(() => { get("pdf"); })}>PDF</button>
        <button type="button" onClick={pick(() => { get("docx"); })}>Word</button>
        {canSend && variant === "menu" && (
          <button type="button" disabled={send.isPending || !inv.contactEmail} onClick={pick(sendNow)}>Send invoice now</button>
        )}
      </>
    );
  };
  const viewer = viewing && (
    <FileViewer title={p.invoiceNumber ? `Invoice ${p.invoiceNumber}` : `Invoice preview: ${p.candidateName} (numbered when sent)`}
      path={`/v1/placements/${p.id}/invoice/file?format=pdf`} type={TYPES.pdf} onClose={() => { setViewing(false); }}
      actions={<>
        <button type="button" onClick={() => { get("pdf"); }}>Download PDF</button>
        <button type="button" onClick={() => { get("docx"); }}>Word</button>
        {canSend && sendButton("")}
      </>} />
  );
  const failed = inv.sendError && !inv.sentAt && <span className="warning small" role="alert">Not emailed: {inv.sendError}</span>;
  if (variant === "menu") {
    if (!extra && !seeFees) return null;
    return (
      <>
        <Menu label="⋯" ariaLabel={`Actions for ${p.candidateName}`}>
          {(close) => <>{extra?.(close)}{items(close)}</>}
        </Menu>
        {viewer}
        <ErrorMessage error={error ?? send.error} />
      </>
    );
  }
  return (
    <div className="placement-invoice">
      <span className="muted small placement-invoice-status">{status}</span>
      {failed}
      {seeFees && <Menu label="Invoice" className="small-button">{items}</Menu>}
      {canSend && sendButton("small-button primary")}
      {viewer}
      <ErrorMessage error={error ?? send.error} />
    </div>
  );
}
