import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { emailTemplatesKey } from "../api/keys";
import { RichEditor } from "../components/clientEmail/RichEditor";
import { TemplatePreview } from "../components/clientEmail/TemplatePreview";
import { atCursor, plainToHtml, showBanners, withoutCaretMarks } from "../components/clientEmail/richText";
import { useBanners } from "../components/clientEmail/useBanners";
import { ErrorMessage } from "../components/ErrorMessage";
import { SaveError } from "../components/SaveError";
import { fieldErrors } from "../components/forms";

type Template = Schemas["EmailTemplate"];
type Field = Schemas["EmailField"];

/** The company's email templates, formatted as CV Studio's email client does: change them, or add one. */
export function EmailTemplates({ canManage, canCreate = canManage, canPreview = false }: { canManage: boolean; canCreate?: boolean; canPreview?: boolean }) {
  const list = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const { banners, urls, upload } = useBanners();
  const [editing, setEditing] = useState<string | null>(null); // a template id, or "new"
  const fields = list.data?.fields ?? [];
  const editor = (template?: Template) => (
    <TemplateForm key={template?.id ?? "new"} template={template} canPreview={canPreview} fields={fields} banners={banners} bannerUrls={urls}
      onUploadBanner={(f) => { upload.mutate(f); }} onDone={() => { setEditing(null); }} />
  );
  return (
    <main>
      <h2>Email templates</h2>
      <ErrorMessage error={list.error ?? upload.error} />
      <p className="muted">
        Format the message with the toolbar, and put in fields such as the candidate&apos;s name or expected salary with
        &ldquo;Insert field&rdquo;: they are filled in from the candidate when you write an email (salary only for those who may see it).
      </p>
      {list.data?.items.map((t) =>
        editing === t.id ? editor(t) : (
          <section key={t.id} className="panel" aria-label={t.name}>
            <div className="row spread">
              <strong>{t.name}</strong>
              {canManage && <button type="button" className="link" onClick={() => { setEditing(t.id); }}>Change</button>}
            </div>
            <p className="muted">{t.subject}</p>
            {t.bodyHtml ? <FormattedBody html={t.bodyHtml} bannerUrls={urls} /> : <p className="email-body">{t.body}</p>}
          </section>
        ),
      )}
      {!canManage && (
        <p className="muted small">Only admins change these templates (Settings, Roles and access: &ldquo;Change and delete the company&apos;s email
          templates&rdquo;). {canCreate ? "You can add new ones, here or with \u201cSave as a new template\u201d when writing an email." : ""}</p>
      )}
      {canCreate && (editing === "new" ? editor() : <button type="button" onClick={() => { setEditing("new"); }}>Add template</button>)}
    </main>
  );
}

/** A saved template's message as it was formatted (the server keeps only safe formatting). */
function FormattedBody({ html, bannerUrls }: { html: string; bannerUrls: Record<string, string> }) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => { showBanners(box.current, bannerUrls); }, [html, bannerUrls]);
  return <div ref={box} className="email-body formatted" dangerouslySetInnerHTML={{ __html: html }} />;
}

type FormProps = {
  template?: Template;
  canPreview: boolean;
  fields: Field[];
  banners: Schemas["EmailBanner"][];
  bannerUrls: Record<string, string>;
  onUploadBanner: (file: File) => void;
  onDone: () => void;
};

function TemplateForm({ template, canPreview, fields, banners, bannerUrls, onUploadBanner, onDone }: FormProps) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: template?.name ?? "", subject: template?.subject ?? "" });
  const [html, setHtml] = useState(() => template?.bodyHtml ?? plainToHtml(template?.body ?? ""));
  const subject = useRef<HTMLInputElement>(null);
  const save = useMutation({
    mutationFn: async () => {
      const body = { ...form, bodyHtml: withoutCaretMarks(html) };
      return template
        ? unwrap(await api.PUT("/v1/email-templates/{templateId}", { params: { path: { templateId: template.id } }, body: { version: template.version, ...body } }))
        : unwrap(await api.POST("/v1/email-templates", { body }));
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: emailTemplatesKey });
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const empty = !html.replace(/<[^>]*>/g, "").replace(/&nbsp;/g, " ").trim();
  // "Insert field" puts a field where the cursor last was: the subject or the message.
  const inSubject = useRef(false);
  return (
    <form className="panel form-grid" aria-label={template ? `Change ${template.name}` : "New template"} onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <label className="field full">Name
        <input value={form.name} onChange={(e) => { setForm({ ...form, name: e.target.value }); }} />
        {errors.name && <span className="warning">{errors.name}</span>}
      </label>
      <label className="field full">Subject
        <input ref={subject} value={form.subject} onFocus={() => { inSubject.current = true; }}
          onChange={(e) => { setForm({ ...form, subject: e.target.value }); }} />
        {errors.subject && <span className="warning">{errors.subject}</span>}
      </label>
      <p className="muted small full">&ldquo;Insert field&rdquo; puts a field where your cursor is: in the subject or the message.</p>
      <div className="full">
        <RichEditor html={html} resetKey={0} onChange={setHtml} fields={fields} banners={banners} bannerUrls={bannerUrls}
          onUploadBanner={onUploadBanner} onFocus={() => { inSubject.current = false; }}
          insertElsewhere={(f) => {
            if (!inSubject.current) return false;
            setForm((cur) => ({ ...cur, subject: atCursor(subject.current, cur.subject, f.token) }));
            return true;
          }} />
        {(errors.body ?? errors.bodyHtml) && <span className="warning">{errors.body ?? errors.bodyHtml}</span>}
      </div>
      {canPreview && <TemplatePreview subject={form.subject} html={html} bannerUrls={bannerUrls} />}
      <SaveError error={save.error} shown={["name", "subject", "body", "bodyHtml"]}
        onReload={() => { void queryClient.invalidateQueries({ queryKey: emailTemplatesKey }); onDone(); }} />
      <div className="row full">
        <button type="submit" disabled={save.isPending || !form.name.trim() || !form.subject.trim() || empty}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}
