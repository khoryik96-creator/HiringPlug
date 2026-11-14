// One email to several candidates or contacts, each their own copy: to them
// alone, fields such as [First name] filled from them, kept on their emails.
// A check says who gets it and who does not (and why) before sending.
import { MailDeliveryNotice } from "./MailDeliveryNotice";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { candidateEmailsKey, contactEmailsKey, emailTemplatesKey } from "../api/keys";
import { Attachments } from "./clientEmail/Attachments";
import { RichEditor } from "./clientEmail/RichEditor";
import { plainToHtml, withoutCaretMarks } from "./clientEmail/richText";
import { useBanners } from "./clientEmail/useBanners";
import { filledIn } from "../requiredFields";
import { ErrorMessage } from "./ErrorMessage";

/** Someone to write to: a candidate or a client's contact. */
export type Person = { candidateId?: string; contactId?: string; name: string; email?: string | null };

const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;
const idOf = (p: Person) => p.candidateId ?? p.contactId ?? "";
const recipientOf = (p: Person) => (p.candidateId ? { candidateId: p.candidateId } : { contactId: p.contactId });

export function SeparateEmail({ people: chosen, onClose }: { people: Person[]; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [people, setPeople] = useState(chosen);
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const { banners, urls } = useBanners();
  const [picked, setPicked] = useState("");
  const [subject, setSubject] = useState("");
  const subjectField = useRef<HTMLInputElement>(null);
  const [html, setHtml] = useState("");
  const [reset, setReset] = useState(0);
  const [uploads, setUploads] = useState<Schemas["EmailAttachment"][]>([]);
  const [as, setAs] = useState(idOf(chosen[0] ?? { name: "" }));
  const message = () => ({ subject, bodyHtml: withoutCaretMarks(html), timeZone: timeZone() });
  const batch = (checkOnly: boolean) => ({
    ...message(), recipients: people.map(recipientOf), attachmentIds: uploads.map((u) => u.id), checkOnly,
  });

  const check = useMutation({ mutationFn: async () => unwrap(await api.POST("/v1/email-batches", { body: batch(true) })) });
  const send = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/email-batches", { body: batch(false) })),
    onSuccess: async () => {
      for (const p of people) {
        await queryClient.invalidateQueries({ queryKey: p.candidateId ? candidateEmailsKey(p.candidateId) : contactEmailsKey(p.contactId ?? "") });
      }
    },
  });
  const previewAs = people.find((p) => idOf(p) === as) ?? people[0];
  const preview = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/email-previews", { body: { ...message(), ...(previewAs ? recipientOf(previewAs) : {}) } })),
  });
  const result = send.data ?? check.data;
  const selected = templates.data?.items.find((t) => t.id === picked);

  if (send.data) {
    const missed = send.data.recipients.filter((r) => r.problem);
    return (
      <section className="panel separate-email" aria-label="Email each separately">
        <h3>Queued for {send.data.sent} {send.data.sent === 1 ? "person" : "people"}, each their own copy</h3>
        <MailDeliveryNotice/>
        {missed.length > 0 && <RecipientTable rows={missed} title="Not sent to" />}
        <p className="muted small">Each email is on that person&apos;s emails; they go out in a moment.</p>
        <button type="button" onClick={onClose}>Close</button>
      </section>
    );
  }
  return (
    <section className="panel separate-email" aria-label="Email each separately">
      <div className="row spread">
        <h3>Email {people.length} {people.length === 1 ? "person" : "people"} separately</h3>
        <button type="button" onClick={onClose}>Close</button>
      </div>
      <MailDeliveryNotice/>
      <p className="muted small">Each gets their own copy, to them alone: nobody sees the others. Fields such as [First name] are filled for each person.</p>
      <ul className="people-chips" aria-label="Recipients">
        {people.map((p) => (
          <li key={idOf(p)} className="person-chip">
            <span>{p.name}{p.email ? <span className="muted"> · {p.email}</span> : p.email === null && <span className="warning"> · no email</span>}</span>
            {people.length > 1 && (
              <button type="button" aria-label={`Leave out ${p.name}`} onClick={() => { setPeople(people.filter((x) => x !== p)); check.reset(); }}>×</button>
            )}
          </li>
        ))}
      </ul>

      <div className="row">
        <label className="row grow">Template
          <select value={picked} onChange={(e) => { setPicked(e.target.value); }}>
            <option value="">No template</option>
            {templates.data?.items.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
        </label>
        <button type="button" disabled={!selected} onClick={() => {
          if (!selected) return;
          setSubject(selected.subject);
          setHtml(selected.bodyHtml ?? plainToHtml(selected.body));
          setReset((n) => n + 1);
        }}>Use selected template</button>
      </div>
      <label className="field">Subject<input ref={subjectField} required value={subject} onChange={(e) => { setSubject(e.target.value); }} /></label>
      <RichEditor compact html={html} resetKey={reset} onChange={setHtml} fields={templates.data?.fields ?? []} banners={banners} bannerUrls={urls} />
      <Attachments candidateId="" canSeeCvs={false} cvs={[]} onCvs={() => undefined} uploads={uploads} onUploads={setUploads} />

      <div className="row separate-actions">
        <label className="row">Preview as
          <select value={as} onChange={(e) => { setAs(e.target.value); preview.reset(); }}>
            {people.map((p) => <option key={idOf(p)} value={idOf(p)}>{p.name}</option>)}
          </select>
        </label>
        <button type="button" disabled={preview.isPending} onClick={() => { if (filledIn(subjectField.current)) preview.mutate(); }}>Preview</button>
        <button type="button" disabled={check.isPending} onClick={() => { if (filledIn(subjectField.current)) check.mutate(); }}>Check who gets it</button>
        <button type="button" className="primary" disabled={send.isPending} onClick={() => { if (filledIn(subjectField.current)) send.mutate(); }}>
          Send to each separately
        </button>
      </div>
      <ErrorMessage error={preview.error ?? check.error ?? send.error} />
      {result && <RecipientTable rows={result.recipients} title="Who gets it" />}
      {preview.data && (
        <div className="email-preview" aria-label="Preview">
          {preview.data.missing.length > 0 && <p className="warning">No value for {previewAs?.name}: {preview.data.missing.join(", ")}. They will be left out unless it is filled in or taken out.</p>}
          <p className="muted small">To {previewAs?.name}{previewAs?.email ? ` <${previewAs.email}>` : ""}</p>
          <p><strong>{preview.data.subject}</strong></p>
          {/* The server's preview: made safe by it (only the toolbar's formatting is kept). */}
          <div dangerouslySetInnerHTML={{ __html: preview.data.bodyHtml }} />
        </div>
      )}
    </section>
  );
}

function RecipientTable({ rows, title }: { rows: Schemas["EmailBatchRecipient"][]; title: string }) {
  return (
    <table className="recipients-table" aria-label={title}>
      <thead><tr><th>{title}</th><th>Address</th><th>Status</th></tr></thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.candidateId ?? r.contactId ?? r.name}>
            <td>{r.name}</td>
            <td>{r.to ?? "—"}</td>
            <td>{r.problem ? <span className="warning">✗ {r.problem}</span> : <span className="ok">✓ {r.emailId ? "queued" : "gets it"}</span>}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** "3 chosen · Email each separately · Clear", over a list with ticks. */
export function PickBar({ count, onEmail, onClear, children }: { count: number; onEmail: () => void; onClear: () => void; children?: React.ReactNode }) {
  if (count === 0) return null;
  return (
    <div className="pick-bar" role="status">
      <span>{count} chosen</span>
      {children}
      <button type="button" className={children ? undefined : "primary"} disabled={count > 50} onClick={onEmail}>Email each separately</button>
      <button type="button" className="link" onClick={onClear}>Clear</button>
      {count > 50 && <span className="warning small">At most 50 at a time.</span>}
    </div>
  );
}
