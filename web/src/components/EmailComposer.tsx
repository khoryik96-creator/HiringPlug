import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, unwrap } from "../api/client";
import { candidateEmailsKey, emailTemplatesKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { SaveError } from "./SaveError";

/** "a@x.com, b@y.com; c@z.com" -> the addresses. */
export const addresses = (text: string) => text.split(/[\s,;]+/).filter(Boolean);

/** The sender's time zone, so interview times read as they will be met. */
const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

type Props = {
  candidateId: string;
  /** The candidate's jobs, to fill a template for one of them. */
  applications?: { id: string; label: string }[];
  applicationId?: string;
  interviewId?: string;
  /** The template to start with, by name (e.g. "Interview invitation"). */
  template?: string;
  canManageTemplates: boolean;
  onDone: () => void;
};

/**
 * Write an email: pick a template (filled from the candidate, job and
 * interview), check and change it, send. Placeholders without a value stay in
 * brackets, and sending waits until they are filled in.
 */
export function EmailComposer({ candidateId, applications = [], applicationId, interviewId, template, canManageTemplates, onDone }: Props) {
  const queryClient = useQueryClient();
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const [about, setAbout] = useState(applicationId ?? "");
  const [form, setForm] = useState({ templateId: "", to: "", cc: "", subject: "", body: "" });
  const [missing, setMissing] = useState<string[]>([]);

  const draft = useMutation({
    mutationFn: async ({ templateId, applicationId: app }: { templateId: string; applicationId: string }) =>
      unwrap(await api.POST("/v1/email-drafts", {
        body: { templateId, candidateId, applicationId: app || undefined, interviewId, timeZone: timeZone() },
      })),
    onSuccess: (d, { templateId }) => {
      setForm((f) => ({ ...f, templateId, to: d.to.join(", "), subject: d.subject, body: d.body }));
      setMissing(d.missing);
    },
  });
  const pick = (templateId: string, app = about) => {
    setForm((f) => ({ ...f, templateId }));
    if (templateId) draft.mutate({ templateId, applicationId: app });
  };

  // Start with the template asked for, once the templates are loaded.
  const started = useRef(false);
  useEffect(() => {
    const t = templates.data?.items.find((x) => x.name === template);
    if (started.current || !t) return;
    started.current = true;
    pick(t.id);
  });

  const send = useMutation({
    mutationFn: async () =>
      unwrap(await api.POST("/v1/emails", {
        body: {
          to: addresses(form.to), cc: addresses(form.cc), subject: form.subject, body: form.body,
          candidateId, applicationId: about || undefined,
        },
      })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: candidateEmailsKey(candidateId) });
      onDone();
    },
  });
  const unfilled = missing.filter((key) => `${form.subject}\n${form.body}`.includes(`[${key}]`));
  const set = (field: keyof typeof form) => (e: { target: { value: string } }) => { setForm({ ...form, [field]: e.target.value }); };

  return (
    <form className="panel form-grid" aria-label="Write email" onSubmit={(e) => { e.preventDefault(); send.mutate(); }}>
      <ErrorMessage error={templates.error ?? draft.error} />
      <label className="field">Template
        <select value={form.templateId} onChange={(e) => { pick(e.target.value); }}>
          <option value="">No template</option>
          {templates.data?.items.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>
      </label>
      {applications.length > 0 && !interviewId && (
        <label className="field">About
          <select value={about} onChange={(e) => { setAbout(e.target.value); if (form.templateId) pick(form.templateId, e.target.value); }}>
            <option value="">No particular job</option>
            {applications.map((a) => <option key={a.id} value={a.id}>{a.label}</option>)}
          </select>
        </label>
      )}
      <label className="field full">To<input value={form.to} onChange={set("to")} /></label>
      <label className="field full">Cc<input value={form.cc} onChange={set("cc")} /></label>
      <label className="field full">Subject<input value={form.subject} onChange={set("subject")} /></label>
      <label className="field full">Message<textarea rows={12} value={form.body} onChange={set("body")} /></label>
      {unfilled.length > 0 && (
        <p className="full warning" role="status">
          Fill in before sending: {unfilled.map((k) => `[${k}]`).join(", ")}
        </p>
      )}
      <SaveError error={send.error} shown={[]} onReload={onDone} />
      <div className="row full">
        <button type="submit" disabled={send.isPending || draft.isPending || unfilled.length > 0 || !form.to.trim() || !form.subject.trim() || !form.body.trim()}>
          Send
        </button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
        {canManageTemplates && <a href="#/email-templates" className="muted">Edit templates</a>}
      </div>
    </form>
  );
}
