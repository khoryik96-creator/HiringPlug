// Settings › WhatsApp templates: the messages recruiters start from on a
// candidate's or contact's WhatsApp button, with fields such as
// {{candidate.firstName}} filled for them (never salary).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { whatsAppTemplatesKey } from "../components/WhatsApp";

type Template = Schemas["WhatsAppTemplate"];

function TemplateForm({ initial, placeholders, saving, error, onSave, onCancel, onDelete }: {
  initial: { name: string; body: string }; placeholders: string[]; saving: boolean; error: unknown;
  onSave: (v: { name: string; body: string }) => void; onCancel?: () => void; onDelete?: () => void;
}) {
  const [name, setName] = useState(initial.name);
  const [body, setBody] = useState(initial.body);
  return (
    <form className="form-grid" onSubmit={(e) => { e.preventDefault(); onSave({ name, body }); }}>
      <label className="field">Name<input required maxLength={100} value={name} onChange={(e) => { setName(e.target.value); }} /></label>
      <label className="field full">Message<textarea required rows={4} maxLength={4000} value={body} onChange={(e) => { setBody(e.target.value); }} /></label>
      <div className="full whatsapp-fields" aria-label="Fields">
        {placeholders.map((p) => (
          <button key={p} type="button" className="chip" onClick={() => { setBody(`${body}{{${p}}}`); }}>{`{{${p}}}`}</button>
        ))}
      </div>
      <ErrorMessage error={error} />
      <div className="row full">
        <button type="submit" disabled={saving}>Save</button>
        {onCancel && <button type="button" className="link" onClick={onCancel}>Cancel</button>}
        {onDelete && <button type="button" className="link danger" disabled={saving} onClick={onDelete}>Delete</button>}
      </div>
    </form>
  );
}

export function WhatsAppTemplates({ canCreate, canManage }: { canCreate: boolean; canManage: boolean }) {
  const cache = useQueryClient();
  const list = useQuery({ queryKey: whatsAppTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/whatsapp-templates")) });
  const [editing, setEditing] = useState<string | null>(null); // a template's id, or "new"
  const done = async () => { setEditing(null); await cache.invalidateQueries({ queryKey: whatsAppTemplatesKey }); };
  const create = useMutation({ mutationFn: async (v: { name: string; body: string }) => unwrap(await api.POST("/v1/whatsapp-templates", { body: v })), onSuccess: done });
  const update = useMutation({
    mutationFn: async ({ t, v }: { t: Template; v: { name: string; body: string } }) =>
      unwrap(await api.PUT("/v1/whatsapp-templates/{templateId}", { params: { path: { templateId: t.id } }, body: { version: t.version, ...v } })),
    onSuccess: done,
  });
  const remove = useMutation({
    mutationFn: async (t: Template) => { unwrap(await api.DELETE("/v1/whatsapp-templates/{templateId}", { params: { path: { templateId: t.id } } })); },
    onSuccess: done,
  });
  const placeholders = list.data?.placeholders ?? [];
  return (
    <main className="page">
      <div className="row spread">
        <h2>WhatsApp templates</h2>
        {canCreate && editing !== "new" && <button type="button" onClick={() => { setEditing("new"); }}>Add a template</button>}
      </div>
      <p className="muted">Start a WhatsApp message from a candidate's or contact's page. Fields are filled for them; salary is never filled.</p>
      <ErrorMessage error={list.error ?? remove.error} />
      {editing === "new" && (
        <section className="panel" aria-label="New template">
          <TemplateForm initial={{ name: "", body: "" }} placeholders={placeholders} saving={create.isPending} error={create.error}
            onSave={(v) => { create.mutate(v); }} onCancel={() => { setEditing(null); }} />
        </section>
      )}
      {list.data?.items.map((t) => (
        <section key={t.id} className="panel whatsapp-template" aria-label={t.name}>
          {editing === t.id ? (
            <TemplateForm initial={t} placeholders={placeholders} saving={update.isPending || remove.isPending} error={update.error}
              onSave={(v) => { update.mutate({ t, v }); }} onCancel={() => { setEditing(null); }}
              onDelete={() => { if (window.confirm(`Delete the "${t.name}" template?`)) remove.mutate(t); }} />
          ) : (
            <>
              <div className="row spread">
                <h3>{t.name}</h3>
                {canManage && <button type="button" className="link" onClick={() => { setEditing(t.id); }}>Change</button>}
              </div>
              <p className="whatsapp-bubble">{t.body}</p>
            </>
          )}
        </section>
      ))}
    </main>
  );
}
