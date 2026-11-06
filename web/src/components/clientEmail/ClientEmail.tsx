// Emailing the client about a candidate, laid out like CV Studio's composer:
// templates (saved with the subject, the formatted message, its signature
// and banner), To / CC / BCC, a formatted message with fields such as
// [Expected salary] showing the candidate's value at once (editable, the
// record untouched), and attachments.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../../api/client";
import { candidateEmailsKey, emailTemplatesKey } from "../../api/keys";
import { addresses } from "../EmailComposer";
import { ErrorMessage } from "../ErrorMessage";
import { fieldErrors } from "../forms";
import { Attachments, type CvChoice } from "./Attachments";
import { RichEditor } from "./RichEditor";
import { plainToHtml, showBanners, withoutCaretMarks } from "./richText";
import { useBanners } from "./useBanners";

const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

type Props = { candidateId: string; applicationId?: string; permissions: Permission[]; onClose: () => void };

export function ClientEmail({ candidateId, applicationId, permissions, onClose }: Props) {
  const queryClient = useQueryClient();
  const canManage = permissions.includes("emails.templates:manage");
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const { banners, urls, upload: uploadBanner } = useBanners();
  const [picked, setPicked] = useState("");
  const [name, setName] = useState("");
  const [form, setForm] = useState({ cc: "", bcc: "", subject: "" });
  const [typedTo, setTypedTo] = useState<string | null>(null); // null: the job's hiring contact
  const [html, setHtml] = useState("");
  const [reset, setReset] = useState(0);
  const [cvs, setCvs] = useState<CvChoice[]>([]);
  const [uploads, setUploads] = useState<Schemas["EmailAttachment"][]>([]);
  const [preview, setPreview] = useState<Schemas["EmailPreview"] | null>(null);
  const [sent, setSent] = useState(false);
  const previewBox = useRef<HTMLDivElement>(null);

  // The job's hiring contact, to send to, and the candidate's value for each field.
  const context = useQuery({
    queryKey: ["email-context", applicationId ?? candidateId],
    queryFn: async () => unwrap(await api.POST("/v1/email-previews", {
      body: { subject: "", bodyHtml: "", candidateId, applicationId, timeZone: timeZone() },
    })),
  });
  const to = typedTo ?? context.data?.clientEmail ?? "";
  useEffect(() => { showBanners(previewBox.current, urls); }, [preview, urls]);

  const selected = templates.data?.items.find((t) => t.id === picked);
  const useTemplate = () => {
    if (!selected) return;
    setForm((f) => ({ ...f, subject: selected.subject }));
    setHtml(selected.bodyHtml ?? plainToHtml(selected.body));
    setName(selected.name);
    setReset((n) => n + 1);
  };
  const saveTemplate = useMutation({
    mutationFn: async (asNew: boolean) => {
      const body = { name: name.trim(), subject: form.subject, bodyHtml: withoutCaretMarks(html) };
      if (asNew || !selected) return unwrap(await api.POST("/v1/email-templates", { body }));
      return unwrap(await api.PUT("/v1/email-templates/{templateId}", { params: { path: { templateId: selected.id } }, body: { ...body, version: selected.version } }));
    },
    onSuccess: async (t) => { await queryClient.invalidateQueries({ queryKey: emailTemplatesKey }); setPicked(t.id); },
  });
  const deleteTemplate = useMutation({
    mutationFn: async (templateId: string) => { unwrap(await api.DELETE("/v1/email-templates/{templateId}", { params: { path: { templateId } } })); },
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: emailTemplatesKey }); setPicked(""); setName(""); },
  });
  const show = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/email-previews", {
      body: { subject: form.subject, bodyHtml: withoutCaretMarks(html), candidateId, applicationId, timeZone: timeZone() },
    })),
    onSuccess: setPreview,
  });
  const send = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/emails", {
      body: {
        to: addresses(to), cc: addresses(form.cc), bcc: addresses(form.bcc), subject: form.subject, bodyHtml: withoutCaretMarks(html),
        candidateId, applicationId, timeZone: timeZone(), attachmentIds: uploads.map((u) => u.id), cvs,
      },
    })),
    onSuccess: async () => { setSent(true); await queryClient.invalidateQueries({ queryKey: candidateEmailsKey(candidateId) }); },
  });
  const errors = fieldErrors(send.error);
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => { setForm((f) => ({ ...f, [k]: e.target.value })); };

  if (sent) {
    return (
      <section className="panel" aria-label="Email client">
        <p role="status">Sent. It is on the candidate&apos;s emails, and goes out in a moment.</p>
        <button type="button" onClick={onClose}>Close</button>
      </section>
    );
  }
  return (
    <section className="panel client-email" aria-label="Email client">
      <div className="row spread"><h3>Email client</h3><button type="button" onClick={onClose}>Close</button></div>
      <p>From: {me.data ? `${me.data.user.name} (${me.data.tenant.name}) · replies go to ${me.data.user.email}` : "you"}</p>

      <div className="row">
        <label className="row grow">Template
          <select value={picked} onChange={(e) => { setPicked(e.target.value); }}>
            <option value="">No template</option>
            {templates.data?.items.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
        </label>
        <button type="button" disabled={!selected} onClick={useTemplate}>Use selected template</button>
        <button type="button" onClick={() => { void templates.refetch(); }}>Refresh templates</button>
      </div>
      {canManage && (
        <div className="row">
          <label className="row grow">Template name <input value={name} onChange={(e) => { setName(e.target.value); }} /></label>
          <button type="button" disabled={!selected || !name.trim() || saveTemplate.isPending} onClick={() => { saveTemplate.mutate(false); }}>Save changes</button>
          <button type="button" disabled={!name.trim() || saveTemplate.isPending} onClick={() => { saveTemplate.mutate(true); }}>Save as new</button>
          <button type="button" disabled={!selected || deleteTemplate.isPending}
            onClick={() => { if (selected && window.confirm(`Delete the template "${selected.name}"?`)) deleteTemplate.mutate(selected.id); }}>Delete template</button>
        </div>
      )}
      <p className="muted small">Templates save the subject, formatted message, signature and banner.</p>
      <ErrorMessage error={saveTemplate.error ?? deleteTemplate.error} />

      <label className="field">To<input value={to} onChange={(e) => { setTypedTo(e.target.value); }} placeholder="client@company.com" /></label>
      {errors.to && <small className="error">{errors.to}</small>}
      <div className="two">
        <label className="field">CC<input value={form.cc} onChange={set("cc")} /></label>
        <label className="field">BCC<input value={form.bcc} onChange={set("bcc")} /></label>
      </div>
      <label className="field">Subject<input value={form.subject} onChange={set("subject")} placeholder="[Full name] for [Job title]" /></label>

      <RichEditor html={html} resetKey={reset} onChange={setHtml} fields={templates.data?.fields ?? []} banners={banners}
        bannerUrls={urls} onUploadBanner={canManage ? (f) => { uploadBanner.mutate(f); } : undefined}
        values={context.data?.fieldValues} />
      <ErrorMessage error={uploadBanner.error} />
      <p className="muted small">
        A field such as [Expected salary] shows the candidate&apos;s value at once (highlighted): edit it as you like for this
        email; their record is not changed. Templates keep the field, not the value.
        {!permissions.includes("candidates.compensation:read") && " Salary fields need a role that may see salaries."}
      </p>

      <Attachments candidateId={candidateId} canSeeCvs={permissions.includes("documents:read")} cvs={cvs} onCvs={setCvs} uploads={uploads} onUploads={setUploads} />

      <div className="row">
        <button type="button" disabled={show.isPending} onClick={() => { show.mutate(); }}>Preview</button>
        <button type="button" className="primary" disabled={send.isPending} onClick={() => { send.mutate(); }}>Send email</button>
      </div>
      <ErrorMessage error={show.error ?? send.error} />
      {preview && (
        <div className="email-preview" aria-label="Preview">
          {preview.missing.length > 0 && <p className="warning">No value for this candidate: {preview.missing.join(", ")}. Fill it in or take it out before sending.</p>}
          {preview.salary && <p className="muted small">It shows salary: only roles that may see salaries will see this email in the app.</p>}
          <p><strong>{preview.subject}</strong></p>
          {/* The server's preview: made safe by it (only the toolbar's formatting is kept). */}
          <div ref={previewBox} dangerouslySetInnerHTML={{ __html: preview.bodyHtml }} />
        </div>
      )}
    </section>
  );
}
