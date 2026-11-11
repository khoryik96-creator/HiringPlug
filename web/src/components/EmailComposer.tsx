import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { candidateEmailsKey, contactEmailsKey, emailTemplatesKey } from "../api/keys";
import { BookingLinkForm } from "./CandidateBookings";
import { Attachments } from "./clientEmail/Attachments";
import { ShortlistPanel, useShortlist } from "./clientEmail/Shortlist";
import { useCandidatePackages } from "./clientEmail/useCandidatePackages";
import { RichEditor } from "./clientEmail/RichEditor";
import { SaveAsTemplate } from "./clientEmail/SaveAsTemplate";
import { atCursor, fillTokens, plainToHtml, showBanners, withoutCaretMarks } from "./clientEmail/richText";
import { useBanners } from "./clientEmail/useBanners";
import { ErrorMessage } from "./ErrorMessage";
import { SaveError } from "./SaveError";

/** "a@x.com, b@y.com; c@z.com" -> the addresses. */
export const addresses = (text: string) => text.split(/[\s,;]+/).filter(Boolean);

/** The sender's time zone, so interview times read as they will be met. */
const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

type Props = {
  /** About a candidate, or (contactId) to one of a client's contacts. */
  candidateId?: string;
  permissions?: Permission[];
  contactId?: string;
  /** Who it goes to before a template says (a contact's address). */
  defaultTo?: string;
  /** The candidate's jobs, to fill a template for one of them. */
  applications?: { id: string; label: string }[];
  applicationId?: string;
  interviewId?: string;
  /** The template to start with, by name (e.g. "Interview invitation"). */
  template?: string;
  canManageTemplates: boolean;
  /** The sender may save the email as a new template (emails.templates:create). */
  canCreateTemplates?: boolean;
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
export function EmailComposer({ candidateId, contactId, defaultTo = "", applications = [], applicationId, interviewId, template, canManageTemplates, canCreateTemplates = false, canBook = false, permissions = [], onDone }: Props) {
  const queryClient = useQueryClient();
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const [about, setAbout] = useState(applicationId ?? "");
  const [form, setForm] = useState({ templateId: "", to: defaultTo, cc: "", bcc: "", subject: "" });
  // Whom the fields are filled from: the candidate (and a job), or the contact (and their company).
  const who = contactId ? { contactId } : { candidateId };
  const [html, setHtml] = useState("");
  const [reset, setReset] = useState(0); // puts html back into the editor (a template)
  const [missing, setMissing] = useState<string[]>([]);
  const packages=useCandidatePackages();
  const list=useShortlist(packages.people,permissions.includes("documents:read"));
  const [uploading,setUploading]=useState(false);
  const previewBox=useRef<HTMLDivElement>(null);
  const [uploads,setUploads]=useState<Schemas["EmailAttachment"][]>([]);
  const [preview,setPreview]=useState<Schemas["EmailPreview"]>();
  const [booking, setBooking] = useState(false);
  const { banners, urls, upload: uploadBanner } = useBanners();
  useEffect(()=>{showBanners(previewBox.current,urls);},[preview,urls]);
  // The candidate's value for each field, so a field picked from the toolbar goes in as its value.
  const context = useQuery({
    queryKey: ["email-context", candidateId ?? "", contactId ?? "", about, interviewId ?? ""],
    queryFn: async () => unwrap(await api.POST("/v1/email-previews", {
      body: { subject: "", bodyHtml: "", ...who, applicationId: about || undefined, interviewId, timeZone: timeZone() },
    })),
  });
  // A booking link to put in where the cursor was in the message (at its end when it had none).
  // "Insert field" puts a field where the cursor last was: the subject or the message.
  const subjectBox = useRef<HTMLInputElement>(null);
  const inSubject = useRef(false);
  const values = context.data?.fieldValues;
  const fields = templates.data?.fields ?? [];
  // A template's fields in the subject show the candidate's values (the message's are filled by the editor).
  const subject = values ? fillTokens(form.subject, fields, values) : form.subject;
  const insert = (text: string) => {
    packages.insertLoose(plainToHtml(text));
    setBooking(false);
  };

  const draft = useMutation({
    mutationFn: async ({ templateId, applicationId: app }: { templateId: string; applicationId: string }) =>
      unwrap(await api.POST("/v1/email-drafts", {
        body: { templateId, ...who, applicationId: app || undefined, interviewId, timeZone: timeZone() },
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

  const sendContext = packages.people.length>0 ? {candidates:packages.references(html),...(contactId?{contactId}:{recipientCandidateId:candidateId})} : {...who,applicationId:about || undefined};
  const show = useMutation({mutationFn:async()=>unwrap(await api.POST("/v1/email-previews",{body:{subject,bodyHtml:withoutCaretMarks(html),...sendContext,timeZone:timeZone()}})),onSuccess:setPreview});
  const send = useMutation({
    mutationFn: async () => {
      if(uploading || (packages.people.length>0 && (list.loading || list.error))) throw new Error("Wait for the selected CVs and files, or remove the unavailable attachment.");
      return unwrap(await api.POST("/v1/emails", {
        body: {
          to: addresses(form.to), cc: addresses(form.cc), ...(form.bcc.trim()?{bcc:addresses(form.bcc)}:{}), subject, bodyHtml: withoutCaretMarks(html),
          ...sendContext, timeZone: timeZone(), ...(uploads.length?{attachmentIds:uploads.map((u)=>u.id)}:{}), ...(packages.people.length?{cvs:list.cvs}:{}),
        },
      }));
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: contactId ? contactEmailsKey(contactId) : candidateEmailsKey(candidateId ?? "") });
      for(const p of packages.people) void queryClient.invalidateQueries({queryKey:candidateEmailsKey(p.candidateId)});
      onDone();
    },
  });
  const unfilled = missing.filter((key) => `${subject}\n${textOf(html)}`.includes(`[${key}]`));
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
      <label className="field full">Bcc<input value={form.bcc} onChange={set("bcc")} /></label>
      <label className="field full">Subject<input ref={subjectBox} value={subject} onChange={set("subject")}
        onFocus={() => { inSubject.current = true; }} /></label>
      {permissions.includes("candidates:read") && <div className="full">
        <ShortlistPanel people={packages.people} onPeople={packages.change} list={list} canSeeCvs={permissions.includes("documents:read")} canSeeSalary={permissions.includes("candidates.compensation:read")} allowEmpty onRefresh={packages.clear} onInsert={packages.insert}/>
        {candidateId && !packages.people.some((p)=>p.candidateId===candidateId) && packages.people.length<10 && <button type="button" className="link" onClick={()=>{packages.change([...packages.people,{candidateId,name:values?.["[Full name]"] ?? "This candidate",...(about?{applicationId:about}:{})}]);}}>Use this candidate’s details</button>}
        {packages.notice && <p role="status" className="muted small">{packages.notice}</p>}
        <ErrorMessage error={list.error}/>
      </div>}
      <div className="field full">
        <span>Message</span>
        <RichEditor html={html} resetKey={reset} onChange={setHtml} fields={fields} banners={banners}
          bannerUrls={urls} onUploadBanner={canManageTemplates ? (f) => { uploadBanner.mutate(f); } : undefined}
          values={values} inserting={packages.inserting} onFocus={() => { inSubject.current = false; }}
          insertElsewhere={(f, value) => {
            if (!inSubject.current) return false;
            setForm((cur) => ({ ...cur, subject: atCursor(subjectBox.current, subject, value ?? f.token) }));
            return true;
          }} />
      </div>
      {canBook && candidateId && (booking
        ? <div className="full"><BookingLinkForm candidateId={candidateId} applicationId={about || undefined} onInsert={insert} onCancel={() => { setBooking(false); }} /></div>
        : <div className="full"><button type="button" className="link" onClick={() => { setBooking(true); }}>+ Booking link</button></div>)}
      {unfilled.length > 0 && (
        <p className="full warning" role="status">
          Fill in before sending: {unfilled.map((k) => `[${k}]`).join(", ")}
        </p>
      )}
      <div className="full"><Attachments candidateId="" canSeeCvs={false} cvs={[]} onCvs={()=>undefined} uploads={uploads} onUploads={setUploads} onBusy={setUploading} others={list.cvs.length}/></div>
      <ErrorMessage error={show.error}/>
      {preview && <div className="email-preview full" aria-label="Email preview"><strong>{preview.subject}</strong><div ref={previewBox} dangerouslySetInnerHTML={{__html:preview.bodyHtml}}/>{preview.missing.length>0 && <p className="warning">Fill in: {preview.missing.join(", ")}</p>}</div>}
      <SaveError error={send.error} shown={[]} onReload={()=>{packages.clear();void list.refresh();send.reset();}} reloadLabel="Refresh candidate details (your message is kept)" />
      <div className="row full">
        <button type="button" disabled={show.isPending || list.loading} onClick={()=>{show.mutate();}}>Preview email</button>
        <button type="submit" disabled={uploading || (packages.people.length>0 && (list.loading || !!list.error)) || send.isPending || draft.isPending || unfilled.length > 0 || !form.to.trim() || !subject.trim() || !textOf(html)}>
          Send
        </button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
        {canCreateTemplates && <SaveAsTemplate subject={subject} html={html} values={values} />}
        {canManageTemplates && <a href="#/email-templates" className="muted">Edit templates</a>}
      </div>
    </form>
  );
}
