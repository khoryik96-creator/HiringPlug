// "Templates" in the WhatsApp window: add, change or rename, and delete the
// company's WhatsApp templates without leaving the message (the same rights
// as Settings › WhatsApp templates). The Template dropdown follows the changes.
import { useState } from "react";

import type { Schemas } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { useWhatsAppTemplateChanges, useWhatsAppTemplates } from "./useWhatsAppTemplates";
import { WhatsAppTemplateForm } from "./WhatsAppTemplateForm";

type Template = Schemas["WhatsAppTemplate"];

export function WhatsAppTemplateManager({ canCreate, canManage, onRemoved, onBack }: {
  canCreate: boolean; canManage: boolean; onRemoved: (t: Template) => void; onBack: () => void;
}) {
  const list = useWhatsAppTemplates();
  const { create, update, remove } = useWhatsAppTemplateChanges();
  // "new", or the template as it was when its form opened: its version goes
  // with the change, so a change made meanwhile by someone else is not lost.
  const [editing, setEditing] = useState<Template | "new" | null>(null);
  const done = { onSuccess: () => { setEditing(null); } };
  const fields = list.data?.fields ?? [];
  const items = list.data?.items ?? [];
  const busy = update.isPending || remove.isPending;
  const del = (t: Template) => {
    if (!window.confirm(`Delete the "${t.name}" template?`)) return;
    remove.mutate(t, { onSuccess: () => { setEditing(null); onRemoved(t); } });
  };
  return (
    <section className="whatsapp-pane" aria-label="Templates">
      <div className="row spread">
        <h4 tabIndex={-1}>Templates</h4>
        {canCreate && editing !== "new" && <button type="button" onClick={() => { create.reset(); setEditing("new"); }}>Add a template</button>}
      </div>
      <ErrorMessage error={list.error ?? remove.error} />
      {editing === "new" && (
        <div className="whatsapp-template" role="group" aria-label="New template">
          <WhatsAppTemplateForm initial={{ name: "", body: "" }} fields={fields} saving={create.isPending} error={create.error}
            onSave={(v) => { create.mutate(v, done); }} onCancel={() => { setEditing(null); }} />
        </div>
      )}
      {list.isSuccess && items.length === 0 && editing !== "new" && <p className="muted">No WhatsApp templates yet.</p>}
      {items.map((t) => (
        <div key={t.id} className="whatsapp-template" role="group" aria-label={t.name}>
          {editing !== "new" && editing?.id === t.id ? (
            <WhatsAppTemplateForm initial={editing} fields={fields} saving={busy} error={update.error}
              onSave={(v) => { update.mutate({ t: editing, v }, done); }} onCancel={() => { setEditing(null); }} />
          ) : (
            <>
              <div className="row spread">
                <strong>{t.name}</strong>
                {canManage && (
                  <span className="row">
                    <button type="button" className="link" disabled={busy} onClick={() => { update.reset(); setEditing(t); }}>Change or rename</button>
                    <button type="button" className="link danger" disabled={busy} onClick={() => { del(t); }}>Delete</button>
                  </span>
                )}
              </div>
              <p className="whatsapp-bubble">{t.body}</p>
            </>
          )}
        </div>
      ))}
      <div className="row"><button type="button" className="link" onClick={onBack}>Back to the message</button></div>
    </section>
  );
}
