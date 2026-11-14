// Emailing the client about a candidate, laid out like CV Studio's composer:
// templates (saved with the subject, the formatted message, its signature
// and banner), To / CC / BCC, a formatted message with fields such as
// [Expected salary] showing the candidate's value at once (editable, the
// record untouched), and attachments.
import { MailDeliveryNotice, useMailbox } from "../MailDeliveryNotice";
import { EmailRecipients } from "../EmailRecipients";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { emailFieldErrors } from "./emailFieldErrors";
import { api, ApiError, type Permission, type Schemas, unwrap } from "../../api/client";
import { candidateEmailsKey, emailTemplatesKey } from "../../api/keys";
import { addresses } from "../EmailComposer";
import { filledIn } from "../../requiredFields";
import { ErrorMessage } from "../ErrorMessage";

import { Attachments, type CvChoice } from "./Attachments";
import { RichEditor } from "./RichEditor";
import { atCursor, fillTokens, plainToHtml, showBanners, valuesToTokens, withoutCaretMarks } from "./richText";
import { SaveAsTemplate } from "./SaveAsTemplate";
import { type Shortlisted, ShortlistPanel, useShortlist } from "./Shortlist";
import { useCandidatePackages } from "./useCandidatePackages";
import { useBanners } from "./useBanners";

const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

type Props = {
  /** About one candidate (and their application)… */
  candidateId?: string; applicationId?: string;
  /** …or about several: their details go in the message, each one's profile is attached. */
  shortlist?: Shortlisted[]; jobId?: string;
  permissions: Permission[]; onClose: () => void;
};

export function ClientEmail({ candidateId = "", applicationId, shortlist, jobId, permissions, onClose }: Props) {
  const queryClient = useQueryClient();
  const mailbox=useMailbox();
  const canManage = permissions.includes("emails.templates:manage");
  const canCreate = permissions.includes("emails.templates:create");
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const { banners, urls, upload: uploadBanner } = useBanners();
  const [picked, setPicked] = useState("");
  const [name, setName] = useState("");
  const nameField = useRef<HTMLInputElement>(null);
  const [form, setForm] = useState({ cc: "", bcc: "", subject: "" });
  const [typedTo, setTypedTo] = useState<string | null>(null); // null: the job's hiring contact
  const [html, setHtml] = useState("");
  const [reset, setReset] = useState(0);
  const [cvs, setCvs] = useState<CvChoice[]>([]);
  const [uploading,setUploading]=useState(false);
  const [uploads, setUploads] = useState<Schemas["EmailAttachment"][]>([]);
  const [preview, setPreview] = useState<Schemas["EmailPreview"] | null>(null);
  const [sent, setSent] = useState(false);
  const previewBox = useRef<HTMLDivElement>(null);
  const [multi,setMulti]=useState(shortlist !== undefined);
  const packages=useCandidatePackages(shortlist ?? []);
  const people=packages.people;
  const canSeeCvs = permissions.includes("documents:read");
  const list = useShortlist(people, canSeeCvs);
  // Who the email is about: several candidates, or one.
  const about = multi
    ? { candidates: packages.references(html) }
    : { candidateId, applicationId };

  // The job's hiring contact, to send to, and the candidate's value for each field.
  const context = useQuery({
    queryKey: multi ? ["email-context", "shortlist", ...people.map((p) => p.applicationId ?? p.candidateId)] : ["email-context", applicationId ?? candidateId],
    queryFn: async () => unwrap(await api.POST("/v1/email-previews", {
      body: { subject: "", bodyHtml: "", ...about, timeZone: timeZone() },
    })),
  });
  const to = typedTo ?? context.data?.clientEmail ?? "";
  useEffect(() => { showBanners(previewBox.current, urls); }, [preview, urls]);

  const selected = templates.data?.items.find((t) => t.id === picked);
  const values = context.data?.fieldValues;
  const fields = templates.data?.fields ?? [];
  // "Insert field" puts a field where the cursor last was: the subject or the message.
  const subjectBox = useRef<HTMLInputElement>(null);
  const inSubject = useRef(false);
  // A template's fields in the subject show the candidate's values (the message's are filled by the editor).
  const subject = values ? fillTokens(form.subject, fields, values) : form.subject;
  const useTemplate = () => {
    if (!selected) return;
    setForm((f) => ({ ...f, subject: selected.subject }));
    setHtml(selected.bodyHtml ?? plainToHtml(selected.body));
    setName(selected.name);
    setReset((n) => n + 1);
  };
  const saveTemplate = useMutation({
    mutationFn: async (asNew: boolean) => {
      // The candidate's details go back in as fields (the message's are turned back by the server).
      const body = { name: name.trim(), subject: valuesToTokens(subject, values ?? {}), bodyHtml: withoutCaretMarks(html) };
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
      body: { subject, bodyHtml: withoutCaretMarks(html), ...about, timeZone: timeZone() },
    })),
    onSuccess: setPreview,
  });
  const send = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/emails", {
      body: {
        to: addresses(to), cc: addresses(form.cc), bcc: addresses(form.bcc), subject, bodyHtml: withoutCaretMarks(html),
        ...about, timeZone: timeZone(), attachmentIds: uploads.map((u) => u.id), cvs: multi ? list.cvs : cvs,
      },
    })),
    onSuccess: async () => {
      setSent(true);
      for (const id of multi ? people.map((p) => p.candidateId) : [candidateId]) await queryClient.invalidateQueries({ queryKey: candidateEmailsKey(id) });
    },
  });
  const errors = emailFieldErrors(send.error);
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => { setForm((f) => ({ ...f, [k]: e.target.value })); };

  if (sent) {
    return (
      <section className="panel" aria-label="Email client">
        <p role="status">Email queued. It is on {multi ? "each candidate's" : "the candidate's"} emails, and goes out in a moment.</p>
        <button type="button" onClick={onClose}>Close</button>
      </section>
    );
  }
  return (
    <section className="panel client-email" aria-label="Email client">
      <div className="row spread">
        <h3>{multi ? `Email client about ${String(people.length)} candidate${people.length === 1 ? "" : "s"}` : "Email client"}</h3>
        <button type="button" onClick={onClose}>Close</button>
      </div>
      <MailDeliveryNotice/>
      <p>From: {me.data ? `${me.data.user.name} (${me.data.tenant.name}) · replies go to ${mailbox.data?.connected ? mailbox.data.address ?? me.data.user.email : me.data.user.email}` : "you"}</p>

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
        <details><summary>Manage template</summary><div className="row">
          <label className="row grow">Template name <input ref={nameField} required value={name} onChange={(e) => { setName(e.target.value); }} /></label>
          <button type="button" disabled={!selected || saveTemplate.isPending} onClick={() => { if (filledIn(nameField.current)) saveTemplate.mutate(false); }}>Save changes</button>
          <button type="button" disabled={saveTemplate.isPending} onClick={() => { if (filledIn(nameField.current)) saveTemplate.mutate(true); }}>Save as new</button>
          <button type="button" disabled={!selected || deleteTemplate.isPending}
            onClick={() => { if (selected && window.confirm(`Delete the template "${selected.name}"?`)) deleteTemplate.mutate(selected.id); }}>Delete template</button>
        </div></details>
      )}
      {!canManage && canCreate && <div className="row"><SaveAsTemplate subject={subject} html={html} values={values} /></div>}
      <ErrorMessage error={saveTemplate.error ?? deleteTemplate.error} />

      <EmailRecipients to={to} cc={form.cc} bcc={form.bcc} errors={errors}
        onChange={(field, value) => { if (field === "to") setTypedTo(value); else setForm((current) => ({ ...current, [field]: value })); }}/>
      <label className="field">Subject<input ref={subjectBox} aria-invalid={!!errors.subject || undefined} value={subject} onChange={set("subject")}
        onFocus={() => { inSubject.current = true; }} /></label>

      {["cc","bcc","subject","body","bodyHtml"].map((key)=>errors[key] && <p className="error small" key={key}>{key}: {errors[key]}</p>)}
      {!multi && permissions.includes("candidates:read") && <button type="button" className="link" onClick={()=>{packages.change([{candidateId,applicationId,name:values?.["[Full name]"] ?? "This candidate"}]);setMulti(true);}}>Select candidates and details</button>}
      {multi && (
        <>
          <details className="email-candidates"><summary>Candidates and CVs ({people.length})</summary>
          <ShortlistPanel jobId={jobId} people={people} onPeople={packages.change} list={list} canSeeCvs={canSeeCvs} canSeeSalary={permissions.includes("candidates.compensation:read")} onRefresh={packages.clear}
            onInsert={packages.insert} /></details>
          {packages.notice && <p className="muted small" role="status">{packages.notice}</p>}
          <ErrorMessage error={list.error} />
        </>
      )}
      <RichEditor compact invalid={!!(errors.body ?? errors.bodyHtml) || !!preview?.missing.length} html={html} resetKey={reset} onChange={setHtml} fields={fields} banners={banners}
        bannerUrls={urls} onUploadBanner={canManage ? (f) => { uploadBanner.mutate(f); } : undefined}
        values={values} inserting={packages.inserting} onFocus={() => { inSubject.current = false; }}
        insertElsewhere={(f, value) => {
          if (!inSubject.current) return false;
          setForm((cur) => ({ ...cur, subject: atCursor(subjectBox.current, subject, value ?? f.token) }));
          return true;
        }} />
      <ErrorMessage error={uploadBanner.error} />
      {!permissions.includes("candidates.compensation:read") && <p className="muted small">Salary fields need a role that may see salaries.</p>}

      <Attachments candidateId={candidateId} canSeeCvs={!multi && canSeeCvs} others={multi ? list.cvs.length : 0} cvs={multi?[]:cvs} onCvs={setCvs} uploads={uploads} onUploads={setUploads} onBusy={setUploading} />

      <div className="row">
        <button type="button" disabled={show.isPending} onClick={() => { show.mutate(); }}>Preview</button>
        <button type="button" className="primary" disabled={uploading || send.isPending || (multi && (list.loading || !!list.error))} onClick={() => { send.mutate(); }}>Send email</button>
      </div>
      <ErrorMessage error={show.error ?? send.error} />
      {send.error instanceof ApiError && send.error.code==="CONFLICT_STALE_VERSION" && <button type="button" className="link" onClick={()=>{packages.clear();void list.refresh();send.reset();}}>Refresh candidate details (your message is kept)</button>}
      {preview && (
        <div className="email-preview" aria-label="Preview">
          {preview.missing.length > 0 && <p className="warning">No value for this candidate: {preview.missing.join(", ")}. Fill it in or take it out before sending.</p>}
          {preview.salary && <p className="muted small">Shows salary: hidden from roles that may not see salaries.</p>}
          <p><strong>{preview.subject}</strong></p>
          {/* The server's preview: made safe by it (only the toolbar's formatting is kept). */}
          <div ref={previewBox} dangerouslySetInnerHTML={{ __html: preview.bodyHtml }} />
        </div>
      )}
    </section>
  );
}
