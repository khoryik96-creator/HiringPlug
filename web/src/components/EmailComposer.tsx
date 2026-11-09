import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, unwrap } from "../api/client";
import { candidateEmailsKey, emailTemplatesKey } from "../api/keys";
import { BookingLinkForm } from "./CandidateBookings";
import { RichEditor } from "./clientEmail/RichEditor";
import { plainToHtml, withoutCaretMarks } from "./clientEmail/richText";
import { useBanners } from "./clientEmail/useBanners";
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
  /** The sender may put a booking link in (scheduling:use). */
  canBook?: boolean;
  onDone: () => void;
};

/** The words of a formatted message, to tell whether it says anything. */
const textOf = (html: string) => html.replace(/<[^>]*>/g, " ").replace(/&nbsp;/g, " ").trim();

/**
 * Write an email: pick a template (filled from the candidate, job and
 * interview, formatted as written), check and change it with the toolbar
 * (bold, italic, underline, lists, indent, fields such as [Expected salary]),
 * send. Placeholders without a value stay in brackets, and sending waits
 * until they are filled in.
 */
export function EmailComposer({ candidateId, applications = [], applicationId, interviewId, template, canManageTemplates, canBook = false, onDone }: Props) {
  const queryClient = useQueryClient();
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const [about, setAbout] = useState(applicationId ?? "");
  const [form, setForm] = useState({ templateId: "", to: "", cc: "", subject: "" });
  const [html, setHtml] = useState("");
  const [reset, setReset] = useState(0); // puts html back into the editor (a template, a booking link)
  const [missing, setMissing] = useState<string[]>([]);
  const [booking, setBooking] = useState(false);
  const { banners, urls, upload: uploadBanner } = useBanners();
  // The candidate's value for each field, so a field picked from the toolbar goes in as its value.
  const context = useQuery({
    queryKey: ["email-context", candidateId, about, interviewId ?? ""],
    queryFn: async () => unwrap(await api.POST("/v1/email-previews", {
      body: { subject: "", bodyHtml: "", candidateId, applicationId: about || undefined, interviewId, timeZone: timeZone() },
    })),
  });
  /** Puts a booking link's text at the end of the message. */
  const insert = (text: string) => {
    setHtml((h) => `${withoutCaretMarks(h)}${plainToHtml(text)}`);
    setReset((n) => n + 1);
    setBooking(false);
  };

  const draft = useMutation({
    mutationFn: async ({ templateId, applicationId: app }: { templateId: string; applicationId: string }) =>
      unwrap(await api.POST("/v1/email-drafts", {
        body: { templateId, candidateId, applicationId: app || undefined, interviewId, timeZone: timeZone() },
      })),
    onSuccess: (d, { templateId }) => {
      setForm((f) => ({ ...f, templateId, to: d.to.join(", "), subject: d.subject }));
      setHtml(d.bodyHtml ?? plainToHtml(d.body));
      setReset((n) => n + 1);
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
          to: addresses(form.to), cc: addresses(form.cc), subject: form.subject, bodyHtml: withoutCaretMarks(html),
          candidateId, applicationId: about || undefined, timeZone: timeZone(),
        },
      })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: candidateEmailsKey(candidateId) });
      onDone();
    },
  });
  const unfilled = missing.filter((key) => `${form.subject}\n${textOf(html)}`.includes(`[${key}]`));
  const set = (field: keyof typeof form) => (e: { target: { value: string } }) => { setForm({ ...form, [field]: e.target.value }); };

  return (
    <form className="panel form-grid" aria-label="Write email" onSubmit={(e) => { e.preventDefault(); send.mutate(); }}>
      <ErrorMessage error={templates.error ?? draft.error ?? uploadBanner.error} />
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
      <div className="field full">
        <span>Message</span>
        <RichEditor html={html} resetKey={reset} onChange={setHtml} fields={templates.data?.fields ?? []} banners={banners}
          bannerUrls={urls} onUploadBanner={canManageTemplates ? (f) => { uploadBanner.mutate(f); } : undefined}
          values={context.data?.fieldValues} />
      </div>
      {canBook && (booking
        ? <div className="full"><BookingLinkForm candidateId={candidateId} applicationId={about || undefined} onInsert={insert} onCancel={() => { setBooking(false); }} /></div>
        : <div className="full"><button type="button" className="link" onClick={() => { setBooking(true); }}>+ Booking link</button></div>)}
      {unfilled.length > 0 && (
        <p className="full warning" role="status">
          Fill in before sending: {unfilled.map((k) => `[${k}]`).join(", ")}
        </p>
      )}
      <SaveError error={send.error} shown={[]} onReload={onDone} />
      <div className="row full">
        <button type="submit" disabled={send.isPending || draft.isPending || unfilled.length > 0 || !form.to.trim() || !form.subject.trim() || !textOf(html)}>
          Send
        </button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
        {canManageTemplates && <a href="#/email-templates" className="muted">Edit templates</a>}
      </div>
    </form>
  );
}
