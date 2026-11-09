// Settings > Invoicing: the agency's invoice template (who it is from, bank
// details, numbering, tax, the fee line and footer) and the email that
// carries it on a placement's start date, with {{fields}} filled from the
// placement. Clicking a field puts it where the cursor is; the preview shows
// it with example values.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { fieldErrors } from "../components/forms";

type Settings = Schemas["InvoiceSettings"];
type Form = Omit<Settings, "version" | "fields">;
type TextKey = "lineText" | "footer" | "emailSubject" | "emailBody";

const invoiceSettingsKey = ["invoice-settings"] as const;

/** A text with its {{fields}} filled from examples (unknown ones left as written). */
export function fillExample(text: string, fields: Schemas["InvoiceField"][]): string {
  return text.replace(/\{\{\s*([a-zA-Z]+\.[a-zA-Z]+)\s*\}\}/g, (m, key: string) => fields.find((f) => f.key === key)?.example ?? m);
}

export function Invoicing({ canManage }: { canManage: boolean }) {
  const settings = useQuery({ queryKey: invoiceSettingsKey, queryFn: async () => unwrap(await api.GET("/v1/invoice-settings")) });
  return (
    <main>
      <h2>Invoicing</h2>
      <p className="muted">
        Invoices are made from this template and emailed to the client on the candidate&apos;s start date, when the placement says
        so. The fee, payment terms and recipient come from each placement (filled in from the client&apos;s terms of business).
      </p>
      <ErrorMessage error={settings.error} />
      {settings.data && <InvoicingForm key={settings.data.version} settings={settings.data} canManage={canManage} />}
    </main>
  );
}

function InvoicingForm({ settings, canManage }: { settings: Settings; canManage: boolean }) {
  const queryClient = useQueryClient();
  const { version, fields, ...initial } = settings;
  const [f, setF] = useState<Form>(initial);
  // Where a clicked field goes: the last text the user was in, at their cursor.
  const [cursor, setCursor] = useState<{ key: TextKey; start: number | null; end: number | null }>({ key: "emailBody", start: null, end: null });
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/invoice-settings", { body: { ...f, version } })),
    onSuccess: (saved) => { queryClient.setQueryData(invoiceSettingsKey, saved); },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof Form>(k: K, v: Form[K]) => { setF((cur) => ({ ...cur, [k]: v })); };
  const text = (k: keyof Form) => ({
    value: String(f[k]), disabled: !canManage,
    onChange: (e: { target: { value: string } }) => { set(k, e.target.value as never); },
  });
  const remember = (k: TextKey) => (e: { currentTarget: HTMLInputElement | HTMLTextAreaElement }) => {
    setCursor({ key: k, start: e.currentTarget.selectionStart, end: e.currentTarget.selectionEnd });
  };
  const track = (k: TextKey) => ({ onSelect: remember(k), onBlur: remember(k) });
  const insert = (key: string) => {
    const cur = f[cursor.key];
    const start = cursor.start ?? cur.length;
    const end = cursor.end ?? cur.length;
    set(cursor.key, `${cur.slice(0, start)}{{${key}}}${cur.slice(end)}`);
    setCursor({ ...cursor, start: start + key.length + 4, end: start + key.length + 4 });
  };
  const error = (k: string) => errors[k] && <small className="error">{errors[k]}</small>;
  // The example invoice number is the next one.
  const examples = fields.map((x) => (x.key === "invoice.number" ? { ...x, example: `${f.numberPrefix}${String(f.nextNumber).padStart(4, "0")}` } : x));

  return (
    <form className="invoicing-settings" aria-label="Invoice template" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <section className="panel" aria-label="The invoice">
        <h3>The invoice</h3>
        <div className="form-grid">
          <label className="field">From (your company, a line each: name, address, registration and tax numbers)
            <textarea rows={5} {...text("issuer")} />{error("issuer")}
          </label>
          <label className="field">Bank details (how to pay)
            <textarea rows={5} {...text("bankDetails")} />{error("bankDetails")}
          </label>
          <label className="field">Number prefix
            <input maxLength={20} {...text("numberPrefix")} />
          </label>
          <label className="field">Next invoice number
            <input type="number" min={1} value={f.nextNumber} disabled={!canManage} onChange={(e) => { set("nextNumber", Number(e.target.value)); }} />
            <small className="muted">The next invoice is {f.numberPrefix}{String(f.nextNumber).padStart(4, "0")}.</small>
          </label>
          <label className="field">Tax name
            <input maxLength={40} placeholder="SST" {...text("taxLabel")} />
          </label>
          <label className="field">Tax %
            <input inputMode="decimal" {...text("taxPercent")} />{error("taxPercent")}
            <small className="muted">0 for none: no tax line on the invoice.</small>
          </label>
          <label className="field full">The fee line
            <textarea rows={2} {...text("lineText")} {...track("lineText")} />{error("lineText")}
          </label>
          <label className="field full">Footer
            <textarea rows={2} {...text("footer")} {...track("footer")} />{error("footer")}
          </label>
        </div>
      </section>
      <section className="panel" aria-label="The email">
        <h3>The email</h3>
        <div className="form-grid">
          <label className="field full">Subject
            <input maxLength={200} {...text("emailSubject")} {...track("emailSubject")} />{error("emailSubject")}
          </label>
          <label className="field full">Message (the invoice goes as a PDF attachment)
            <textarea rows={8} {...text("emailBody")} {...track("emailBody")} />{error("emailBody")}
          </label>
          <label className="field">Sent at
            <select value={f.sendHour} disabled={!canManage} onChange={(e) => { set("sendHour", Number(e.target.value)); }}>
              {Array.from({ length: 24 }, (_, h) => <option key={h} value={h}>{`${String(h).padStart(2, "0")}:00`} on the start date</option>)}
            </select>
          </label>
        </div>
      </section>
      {canManage && (
        <section className="panel" aria-label="Fields">
          <h3>Fields</h3>
          <p className="muted small">Click one to put it where your cursor is in the fee line, footer, subject or message.</p>
          <div className="chips">
            {fields.map((x) => (
              <button key={x.key} type="button" className="chip" title={`e.g. ${x.example}`} onClick={() => { insert(x.key); }}>{x.label}</button>
            ))}
          </div>
        </section>
      )}
      <section className="panel" aria-label="Preview">
        <h3>Preview</h3>
        <p className="muted small">With example values.</p>
        <p><strong>{fillExample(f.emailSubject, examples)}</strong></p>
        <p className="pre email-preview">{fillExample(f.emailBody, examples)}</p>
        <p className="muted small">Invoice line: <span className="pre">{fillExample(f.lineText, examples)}</span></p>
      </section>
      {canManage && (
        <div className="row">
          <button type="submit" disabled={save.isPending}>Save invoice template</button>
          {save.isSuccess && <span className="muted" role="status">Saved.</span>}
        </div>
      )}
      <ErrorMessage error={save.error} />
    </form>
  );
}
