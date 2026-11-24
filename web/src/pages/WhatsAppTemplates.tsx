// Settings › WhatsApp templates: the messages recruiters start from on a
// candidate's or contact's WhatsApp button, with fields such as
// {{candidate.firstName}} filled for them (never salary).
import { useState } from "react";

import type { Schemas } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { WhatsAppTemplateForm } from "../components/WhatsAppTemplateForm";
import { useWhatsAppTemplateChanges, useWhatsAppTemplates } from "../components/useWhatsAppTemplates";

type Template = Schemas["WhatsAppTemplate"];

export function WhatsAppTemplates({ canCreate, canManage }: { canCreate: boolean; canManage: boolean }) {
  const list = useWhatsAppTemplates();
  const { create, update, remove } = useWhatsAppTemplateChanges();
  // "new", or the template as it was when its form opened: its version goes
  // with the change, so a change made meanwhile by someone else is not lost.
  const [editing, setEditing] = useState<Template | "new" | null>(null);
  const done = { onSuccess: () => { setEditing(null); } };
  const fields = list.data?.fields ?? [];
  return (
    <main className="page">
      <div className="row spread">
        <h2>WhatsApp templates</h2>
        {canCreate && editing !== "new" && <button type="button" onClick={() => { create.reset(); setEditing("new"); }}>Add a template</button>}
      </div>
      <p className="muted">Start a WhatsApp message from a candidate's or contact's page. Fields are filled for them; salary is never filled.</p>
      <ErrorMessage error={list.error ?? remove.error} />
      {editing === "new" && (
        <section className="panel" aria-label="New template">
          <WhatsAppTemplateForm initial={{ name: "", body: "" }} fields={fields} saving={create.isPending} error={create.error}
            onSave={(v) => { create.mutate(v, done); }} onCancel={() => { setEditing(null); }} />
        </section>
      )}
      {list.data?.items.map((t) => (
        <section key={t.id} className="panel whatsapp-template" aria-label={t.name}>
          {editing !== "new" && editing?.id === t.id ? (
            <WhatsAppTemplateForm initial={editing} fields={fields} saving={update.isPending || remove.isPending} error={update.error}
              onSave={(v) => { update.mutate({ t: editing, v }, done); }} onCancel={() => { setEditing(null); }}
              onDelete={() => { if (window.confirm(`Delete the "${t.name}" template?`)) remove.mutate(t, done); }} />
          ) : (
            <>
              <div className="row spread">
                <h3>{t.name}</h3>
                {canManage && <button type="button" className="link" onClick={() => { update.reset(); setEditing(t); }}>Change or rename</button>}
              </div>
              <p className="whatsapp-bubble">{t.body}</p>
            </>
          )}
        </section>
      ))}
    </main>
  );
}
