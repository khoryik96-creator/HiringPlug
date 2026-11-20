import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../../api/client";
import { uploadOutlookDraftFile } from "../../api/files";
import { formatWhen } from "../../labels";
import { CloseIcon, IconButton, RefreshIcon, TrashIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";
import { Field } from "../Field";
import { fieldErrors, FormInvalid } from "../forms";
import { wallTime, zonedInput } from "../calendar/dates";
import { RichEditor } from "../clientEmail/RichEditor";
import { editorText, plainToHtml, withoutCaretMarks } from "../clientEmail/richText";
import { useCandidatePackages } from "../clientEmail/useCandidatePackages";
import { OutlookCandidateDetails } from "./OutlookCandidateDetails";
import { type ComposeContext, OutlookComposeTools, useComposeData } from "./OutlookComposeTools";
import { OutlookDialog } from "./OutlookDialog";
import { maxDraftFileSize, OutlookDraftFiles } from "./OutlookDraftFiles";
import { hasSignature, withSignature } from "./outlookSignature";
import { outlookAddresses } from "./outlookAddresses";

type Draft = Schemas["OutlookDraft"];
type DraftInput = Schemas["OutlookDraftInput"];

export const outlookScheduledKey = ["outlook", "scheduled"];
const draftKey = (id: string | null) => ["outlook", "draft", id];

export function OutlookDraftEditor({ id, onClose, focusMessage = false, fresh = id === null, permissions = [], about = {} }: {
  id: string | null; onClose: (operation?: "send" | "schedule" | "discard") => void; focusMessage?: boolean;
  /** Just created (a new message, reply or forward): the signature goes in. */
  fresh?: boolean;
  permissions?: Permission[];
  /** The candidate (or contact) whose details fill fields, from the page it was opened on. */
  about?: ComposeContext;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const busy = useRef(false);
  const query = useQuery({
    queryKey: draftKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/outlook/drafts/{messageId}", { params: { path: { messageId: id ?? "" } } })),
    enabled: id !== null,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    gcTime: 0,
  });
  return <OutlookDialog className="outlook-draft-dialog" dialogRef={dialog} labelledBy="outlook-draft-heading"
    onCancel={() => { if (!busy.current) onClose(); }}>
    {id && !query.data ? <>
      <div className="row spread">
        <h3 id="outlook-draft-heading">Outlook draft</h3>
        <IconButton label="Close" icon={<CloseIcon />} onClick={() => { onClose(); }} />
      </div>
      <ErrorMessage error={query.error}/>
      {query.isPending && <p className="muted">Opening draft…</p>}
    </> : <DraftFields key={query.dataUpdatedAt} opened={query.data} focusMessage={focusMessage} setBusy={(pending) => { busy.current = pending; }}
      onClose={onClose} fresh={fresh} permissions={permissions} about={about}/>}
  </OutlookDialog>;
}

function inputOf(value: typeof emptyDraft, before?: Draft): DraftInput {
  const bodyHtml = withoutCaretMarks(value.html);
  if (!before) return { to: outlookAddresses(value.to), cc: outlookAddresses(value.cc), bcc: outlookAddresses(value.bcc), subject: value.subject, bodyHtml };
  return {
    ...(value.to !== before.to.join(", ") ? { to: outlookAddresses(value.to) } : {}),
    ...(value.cc !== before.cc.join(", ") ? { cc: outlookAddresses(value.cc) } : {}),
    ...(value.bcc !== before.bcc.join(", ") ? { bcc: outlookAddresses(value.bcc) } : {}),
    ...(value.subject !== before.subject ? { subject: value.subject } : {}),
    ...(value.html !== markupOf(before) && !before.bodyTruncated ? { bodyHtml } : {}),
  };
}

const emptyDraft = { to: "", cc: "", bcc: "", subject: "", html: "" };
const markupOf = (draft: Draft) => draft.bodyHtml ?? (draft.body ? plainToHtml(draft.body) : "");
const formOf = (draft?: Draft) => draft ? {
  to: draft.to.join(", "), cc: draft.cc.join(", "), bcc: draft.bcc.join(", "), subject: draft.subject, html: markupOf(draft),
} : emptyDraft;
type Operation = "save" | "send" | "schedule" | "discard";
type DraftAction = Operation | "reload" | { file: File } | { remove: string };

function DraftFields({ opened, onClose, setBusy, focusMessage, fresh, permissions, about }: {
  opened?: Draft; onClose: (operation?: "send" | "schedule" | "discard") => void; setBusy: (pending: boolean) => void;
  focusMessage: boolean; fresh: boolean; permissions: Permission[]; about: ComposeContext;
}) {
  const client = useQueryClient();
  const [draft, setDraft] = useState(opened);
  const [form, setForm] = useState(() => formOf(opened));
  const [originalReplyText, setOriginalReplyText] = useState(() => focusMessage ? editorText(form.html).replace(/\s+/g, " ") : undefined);
  const [editorReset, setEditorReset] = useState(0);
  const [candidate, setCandidate] = useState<{ id: string; name: string } | null>(null);
  const [aboutPage, setAboutPage] = useState(about);
  const tools = useComposeData(candidate ? { candidateId: candidate.id } : aboutPage);
  // The page's candidate is ready to have their details put in (their name shows once read).
  const packages = useCandidatePackages(about.candidateId ? [{ candidateId: about.candidateId, name: "This candidate" }] : []);
  /** Puts new content in the message (a template, the signature), and shows it. */
  const replaceMessage = (html: string) => { setForm((f) => ({ ...f, html })); setEditorReset((n) => n + 1); };
  // A message just started gets the signature, once (when it has loaded), as Outlook adds it.
  const [signed, setSigned] = useState(!fresh);
  if (!signed && tools.signatureLoaded) {
    setSigned(true);
    if (tools.signature && !hasSignature(form.html)) {
      const html = withSignature(form.html, tools.signature);
      if (focusMessage) setOriginalReplyText(editorText(html).replace(/\s+/g, " "));
      replaceMessage(html);
    }
  }
  const [copies, setCopies] = useState(!!(form.cc || form.bcc));
  const [later, setLater] = useState(false);
  const [notice, setNotice] = useState("");
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const [at, setAt] = useState(() => opened?.schedule ? zonedInput(new Date(opened.schedule.sendAt), zone) : "");
  const plan = draft?.schedule;
  const locked = plan?.status === "confirming" || plan?.status === "uncertain";
  const action = useMutation({
    onMutate: () => { setBusy(true); },
    onSettled: () => { setBusy(false); },
    mutationFn: async (operation: DraftAction) => {
      if (operation === "reload") {
        if (draft) {
          const latest = unwrap(await api.GET("/v1/outlook/drafts/{messageId}", { params: { path: { messageId: draft.messageId } } }));
          setDraft(latest);
          setForm(formOf(latest));
          setEditorReset((value) => value + 1);
          setCopies(Boolean(latest.cc.length || latest.bcc.length));
          setAt(latest.schedule ? zonedInput(new Date(latest.schedule.sendAt), zone) : "");
          setLater(false);
        }
        return "files";
      }
      if (typeof operation !== "string") {
        let opened = draft;
        if ("file" in operation) {
          if (operation.file.size === 0 || operation.file.size > maxDraftFileSize) {
            throw new Error("Choose a non-empty file up to 10 MB.");
          }
          if (!opened) {
            opened = unwrap(await api.POST("/v1/outlook/drafts", { body: inputOf(form) }));
            setDraft(opened);
          }
          setDraft(await uploadOutlookDraftFile(opened.messageId, opened.version, operation.file));
        } else if (opened) {
          setDraft(unwrap(await api.DELETE("/v1/outlook/drafts/{messageId}/attachments/{attachmentId}", {
            params: { path: { messageId: opened.messageId, attachmentId: operation.remove }, query: { version: opened.version } },
          })));
        }
        return "files";
      }
      if (operation === "discard" && draft) {
        unwrap(await api.DELETE("/v1/outlook/drafts/{messageId}", { params: { path: { messageId: draft.messageId }, query: { version: draft.version } } }));
        return operation;
      }
      let sendAt: string | undefined;
      if (operation === "schedule") {
        try {
          const date = wallTime(at, zone);
          if (date <= new Date()) throw new Error("Choose a future date and time.");
          sendAt = date.toISOString();
        } catch (error) {
          throw new FormInvalid({ sendAt: error instanceof Error ? error.message : "Choose a valid send time." });
        }
      }
      const changes = inputOf(form, draft);
      let saved = draft;
      if (!saved) {
        saved = unwrap(await api.POST("/v1/outlook/drafts", { body: changes }));
      } else if (operation === "save" && Object.keys(changes).length) {
        saved = unwrap(await api.PATCH("/v1/outlook/drafts/{messageId}", { params: { path: { messageId: saved.messageId } }, body: { ...changes, version: saved.version } }));
      }
      if (operation === "send" || operation === "schedule") {
        if (!draft) setDraft(saved);
        unwrap(await api.PUT("/v1/outlook/drafts/{messageId}/send", {
          params: { path: { messageId: saved.messageId } },
          body: { draftVersion: saved.version, expectedVersion: saved.schedule?.version ?? 0, sendAt,
            changes: draft && Object.keys(changes).length ? changes : undefined },
        }));
      } else {
        setDraft(saved);
        setForm(formOf(saved));
        setEditorReset((value) => value + 1);
      }
      return operation;
    },
    onSuccess: (operation) => {
      void client.invalidateQueries({ queryKey: ["outlook", "list"] });
      void client.invalidateQueries({ queryKey: outlookScheduledKey });
      if (operation === "save") {
        setNotice("Saved in Outlook Drafts.");
      } else if (operation === "files") {
        void client.invalidateQueries({ queryKey: ["outlook", "attachments"] });
        setNotice("");
      } else {
        onClose(operation);
      }
    },
    onError: (_error, operation) => {
      if (typeof operation !== "string") void client.invalidateQueries({ queryKey: outlookScheduledKey });
    },
  });
  const errors = fieldErrors(action.error);
  const errorFor = (name: string) => errors[name] ?? Object.entries(errors).find(([field]) => field.startsWith(`${name}.`) || field.startsWith(`${name}[`))?.[1];
  const update = (field: keyof typeof form, value: string) => { setForm({ ...form, [field]: value }); setNotice(""); };
  const messageText = editorText(form.html).replace(/\s+/g, " ");
  const readyToSend = !!form.to.trim() && !!form.subject.trim() && !!messageText && messageText !== originalReplyText;
  return <>
    <div className="row spread outlook-draft-head">
      <h3 id="outlook-draft-heading">{draft ? "Outlook draft" : "New Outlook message"}</h3>
      <IconButton label="Close" icon={<CloseIcon />} disabled={action.isPending} onClick={() => { onClose(); }} />
    </div>
    <form className="form-grid" onSubmit={(event) => { event.preventDefault(); action.mutate("save"); }}>
      <fieldset disabled={locked || action.isPending} className="full outlook-draft-fields">
        <Field label="To" error={errorFor("to")}><input value={form.to} onChange={(e) => { update("to", e.target.value); }}/></Field>
        {!copies && <button type="button" className="link" onClick={() => { setCopies(true); }}>Cc / Bcc</button>}
        {copies && <div className="outlook-copy-fields">
          <Field label="Cc" error={errorFor("cc")}><input value={form.cc} onChange={(e) => { update("cc", e.target.value); }}/></Field>
          <Field label="Bcc" error={errorFor("bcc")}><input value={form.bcc} onChange={(e) => { update("bcc", e.target.value); }}/></Field>
        </div>}
        <Field label="Subject" error={errorFor("subject")}><input maxLength={200} value={form.subject} onChange={(e) => { update("subject", e.target.value); }}/></Field>
        {!draft?.bodyTruncated && <OutlookComposeTools data={tools} html={form.html} subject={form.subject} permissions={permissions}
          candidate={candidate} onCandidate={(c) => {
            setCandidate(c);
            if (!c) setAboutPage({});
            else if (!packages.people.some((p) => p.candidateId === c.id) && packages.people.length < 10) packages.change([...packages.people, { candidateId: c.id, name: c.name }]);
          }}
          onInsert={replaceMessage} onSubject={(s) => { update("subject", s); }}
          onSignature={() => { replaceMessage(withSignature(form.html, tools.signature)); }}
          onAttach={(file) => { action.mutate({ file }); }} disabled={locked || action.isPending} />}
        {!draft?.bodyTruncated && permissions.includes("candidates:read") && <OutlookCandidateDetails packages={packages} permissions={permissions} />}
        {draft?.bodyTruncated ? <Field label="Message"><textarea readOnly rows={9} value={draft.body} /></Field> :
          <div className="field">
            <span>Message</span>
            <RichEditor compact iconTools plainPaste focusAtStart={focusMessage || fresh} readOnly={locked || action.isPending} html={form.html} resetKey={editorReset}
              invalid={!!(errorFor("bodyHtml") ?? errorFor("body"))} onChange={(html) => { update("html", html); }}
              fields={tools.fields} values={tools.values} banners={tools.banners} bannerUrls={tools.bannerUrls} inserting={packages.inserting}
              onUploadBanner={permissions.includes("emails.templates:manage") ? (file) => { tools.uploadBanner.mutate(file); } : undefined} />
          </div>}
      </fieldset>
      <OutlookDraftFiles draft={draft} disabled={locked || action.isPending}
        onAdd={(file) => { action.mutate({ file }); }} onRemove={(attachment) => { action.mutate({ remove: attachment.id }); }} />
      {(draft?.bodyTruncated || draft?.webUrl) && <div className="full row outlook-actions small">
        {draft.bodyTruncated && <span className="muted">Edit the full message in Outlook.</span>}
        {draft.webUrl && <a href={draft.webUrl} target="_blank" rel="noreferrer">Open in Outlook</a>}
      </div>}
      {plan && <p className="full small muted">
        {locked ? "Check the send status in Scheduled."
          : plan.status === "needs_review" ? plan.error || "Review the draft and update the schedule."
            : draft.scheduleNeedsReview ? "Outlook changed this draft. Review it and update the schedule."
            : `Scheduled for ${formatWhen(plan.sendAt)}. Saving keeps this time.`}
      </p>}
      {later && !locked && <div className="full row outlook-actions outlook-schedule-time">
        <Field label={`Send at · ${zone}`} error={errorFor("sendAt")}><input type="datetime-local" value={at} onChange={(e) => { setAt(e.target.value); }}/></Field>
        <button type="button" disabled={!at || !readyToSend || action.isPending} onClick={() => { action.mutate("schedule"); }}>{plan ? "Update schedule" : "Schedule email"}</button>
      </div>}
      <div className="full"><ErrorMessage error={action.error}/></div>
      {notice && <p className="full" role="status">{notice}</p>}
      <div className="row full outlook-actions">
        <button type="submit" className="primary" disabled={locked || action.isPending}>Save draft</button>
        {(!plan || plan.status === "needs_review") && <button type="button" disabled={!readyToSend || action.isPending} onClick={() => { action.mutate("send"); }}>Send now</button>}
        <button type="button" disabled={locked || action.isPending} aria-expanded={later} onClick={() => { setLater(!later); }}>
          {draft?.scheduleNeedsReview ? "Review schedule" : plan ? "Change time" : "Send later"}
        </button>
        {draft && <IconButton label="Reload draft" icon={<RefreshIcon />} disabled={action.isPending} onClick={() => { action.mutate("reload"); }} />}
        {draft && !plan && <IconButton label="Discard draft" icon={<TrashIcon />} danger disabled={action.isPending} onClick={() => {
          if (window.confirm("Move this draft to Outlook Deleted Items?")) action.mutate("discard");
        }} />}
      </div>
    </form>
  </>;
}
