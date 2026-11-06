import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { emailTemplatesKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { SaveError } from "../components/SaveError";
import { fieldErrors } from "../components/forms";

type Template = Schemas["EmailTemplate"];

/** The company's email templates: change them, or add one. */
export function EmailTemplates({ canManage }: { canManage: boolean }) {
  const list = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const [editing, setEditing] = useState<string | null>(null); // a template id, or "new"
  return (
    <main>
      <h2>Email templates</h2>
      <ErrorMessage error={list.error} />
      {list.data && (
        <p className="muted">
          Placeholders are filled in when you write an email:{" "}
          {list.data.placeholders.map((p) => <code key={p}>{`{{${p}}}`} </code>)}
        </p>
      )}
      {list.data?.items.map((t) =>
        editing === t.id ? (
          <TemplateForm key={t.id} template={t} onDone={() => { setEditing(null); }} />
        ) : (
          <section key={t.id} className="panel" aria-label={t.name}>
            <div className="row spread">
              <strong>{t.name}</strong>
              {canManage && <button type="button" className="link" onClick={() => { setEditing(t.id); }}>Change</button>}
            </div>
            <p className="muted">{t.subject}</p>
            <p className="email-body">{t.body}</p>
          </section>
        ),
      )}
      {canManage && (editing === "new"
        ? <TemplateForm onDone={() => { setEditing(null); }} />
        : <button type="button" onClick={() => { setEditing("new"); }}>Add template</button>)}
    </main>
  );
}

function TemplateForm({ template, onDone }: { template?: Template; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: template?.name ?? "", subject: template?.subject ?? "", body: template?.body ?? "" });
  const save = useMutation({
    mutationFn: async () => template
      ? unwrap(await api.PUT("/v1/email-templates/{templateId}", { params: { path: { templateId: template.id } }, body: { version: template.version, ...form } }))
      : unwrap(await api.POST("/v1/email-templates", { body: form })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: emailTemplatesKey });
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const field = (name: keyof typeof form, label: string, multiline = false) => (
    <label className="field full">{label}
      {multiline
        ? <textarea rows={10} value={form[name]} onChange={(e) => { setForm({ ...form, [name]: e.target.value }); }} />
        : <input value={form[name]} onChange={(e) => { setForm({ ...form, [name]: e.target.value }); }} />}
      {errors[name] && <span className="warning">{errors[name]}</span>}
    </label>
  );
  return (
    <form className="panel form-grid" aria-label={template ? `Change ${template.name}` : "New template"} onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      {field("name", "Name")}
      {field("subject", "Subject")}
      {field("body", "Message", true)}
      <SaveError error={save.error} shown={["name", "subject", "body"]} onReload={() => { void queryClient.invalidateQueries({ queryKey: emailTemplatesKey }); onDone(); }} />
      <div className="row full">
        <button type="submit" disabled={save.isPending || !form.name.trim() || !form.subject.trim() || !form.body.trim()}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}
