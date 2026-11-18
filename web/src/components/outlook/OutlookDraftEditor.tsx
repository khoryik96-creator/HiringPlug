import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { formatWhen } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { Field } from "../Field";
import { fieldErrors, FormInvalid } from "../forms";
import { wallTime, zonedInput } from "../calendar/dates";

type Draft = Schemas["OutlookDraft"];
type DraftInput = Schemas["OutlookDraftInput"];

export const outlookScheduledKey = ["outlook", "scheduled"];
const draftKey = (id: string | null) => ["outlook", "draft", id];

export function OutlookDraftEditor({ id, onClose }: { id: string | null; onClose: (operation?: "send" | "schedule" | "discard") => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const query = useQuery({
    queryKey: draftKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/outlook/drafts/{messageId}", { params: { path: { messageId: id ?? "" } } })),
    enabled: id !== null,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    gcTime: 0,
  });
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  return <dialog className="panel outlook-draft-dialog" ref={dialog} aria-labelledby="outlook-draft-heading"
    onCancel={(event) => { event.preventDefault(); onClose(); }}>
    {id && !query.data ? <>
      <div className="row spread">
        <h3 id="outlook-draft-heading">Outlook draft</h3>
        <button type="button" onClick={() => { onClose(); }}>Close</button>
      </div>
      <ErrorMessage error={query.error}/>
      {query.isPending && <p className="muted">Opening draft…</p>}
    </> : <DraftFields key={query.dataUpdatedAt} opened={query.data}
      onClose={onClose} onReload={() => { void query.refetch(); }}/>}
  </dialog>;
}

function listFor(value: string): string[] {
  return value.split(/[,;\s]+/).map((part) => part.trim()).filter(Boolean);
}
function inputOf(value: typeof emptyDraft, before?: Draft): DraftInput {
  if (!before) return { to: listFor(value.to), cc: listFor(value.cc), bcc: listFor(value.bcc), subject: value.subject, body: value.body };
  return {
    ...(value.to !== before.to.join(", ") ? { to: listFor(value.to) } : {}),
    ...(value.cc !== before.cc.join(", ") ? { cc: listFor(value.cc) } : {}),
    ...(value.bcc !== before.bcc.join(", ") ? { bcc: listFor(value.bcc) } : {}),
    ...(value.subject !== before.subject ? { subject: value.subject } : {}),
    ...(value.body !== before.body && !before.bodyTruncated ? { body: value.body } : {}),
  };
}

const emptyDraft = { to: "", cc: "", bcc: "", subject: "", body: "" };
type Operation = "save" | "send" | "schedule" | "discard";

function DraftFields({ opened, onClose, onReload }: { opened?: Draft; onClose: (operation?: "send" | "schedule" | "discard") => void; onReload: () => void }) {
  const client = useQueryClient();
  const [draft, setDraft] = useState(opened);
  const [form, setForm] = useState(() => opened ? {
    to: opened.to.join(", "), cc: opened.cc.join(", "), bcc: opened.bcc.join(", "), subject: opened.subject, body: opened.body,
  } : emptyDraft);
  const [copies, setCopies] = useState(!!(form.cc || form.bcc));
  const [later, setLater] = useState(false);
  const [notice, setNotice] = useState("");
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const [at, setAt] = useState(() => opened?.schedule ? zonedInput(new Date(opened.schedule.sendAt), zone) : "");
  const plan = draft?.schedule;
  const locked = plan?.status === "confirming" || plan?.status === "uncertain";
  const action = useMutation({
    mutationFn: async (operation: Operation) => {
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
      } else if (Object.keys(changes).length) {
        saved = unwrap(await api.PATCH("/v1/outlook/drafts/{messageId}", { params: { path: { messageId: saved.messageId } }, body: { ...changes, version: saved.version } }));
      }
      setDraft(saved);
      setForm({ to: saved.to.join(", "), cc: saved.cc.join(", "), bcc: saved.bcc.join(", "), subject: saved.subject, body: saved.body });
      if (operation === "send" || operation === "schedule") {
        unwrap(await api.PUT("/v1/outlook/drafts/{messageId}/send", {
          params: { path: { messageId: saved.messageId } },
          body: { draftVersion: saved.version, expectedVersion: saved.schedule?.version ?? 0, sendAt },
        }));
      }
      return operation;
    },
    onSuccess: (operation) => {
      void client.invalidateQueries({ queryKey: ["outlook", "list"] });
      void client.invalidateQueries({ queryKey: outlookScheduledKey });
      if (operation === "save") {
        setNotice("Saved in Outlook Drafts.");
      } else {
        onClose(operation);
      }
    },
  });
  const errors = fieldErrors(action.error);
  const errorFor = (name: string) => errors[name] ?? Object.entries(errors).find(([field]) => field.startsWith(`${name}.`) || field.startsWith(`${name}[`))?.[1];
  const update = (field: keyof typeof form, value: string) => { setForm({ ...form, [field]: value }); setNotice(""); };
  const readyToSend = !!form.to.trim() && !!form.subject.trim() && !!form.body.trim();
  return <>
    <div className="row spread outlook-draft-head">
      <h3 id="outlook-draft-heading">{draft ? "Outlook draft" : "New Outlook message"}</h3>
      <button type="button" disabled={action.isPending} onClick={() => { onClose(); }}>Close</button>
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
        <Field label="Message" error={errorFor("body")}><textarea readOnly={draft?.bodyTruncated} maxLength={40000} rows={9} value={form.body} onChange={(e) => { update("body", e.target.value); }}/></Field>
      </fieldset>
      {(draft?.hasAttachments || draft?.bodyTruncated || draft?.webUrl) && <div className="full row outlook-actions small">
        {draft.hasAttachments && <span className="muted">Keeps the attachments in Outlook.</span>}
        {draft.bodyTruncated && <span className="muted">Edit the full message in Outlook.</span>}
        {draft.webUrl && <a href={draft.webUrl} target="_blank" rel="noreferrer">Open in Outlook</a>}
      </div>}
      {plan && <p className="full small muted">
        {locked ? "Check the send status in Scheduled."
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
        {!plan && <button type="button" disabled={!readyToSend || action.isPending} onClick={() => { action.mutate("send"); }}>Send now</button>}
        <button type="button" disabled={locked || action.isPending} aria-expanded={later} onClick={() => { setLater(!later); }}>
          {draft?.scheduleNeedsReview ? "Review schedule" : plan ? "Change time" : "Send later"}
        </button>
        {opened && <button type="button" disabled={action.isPending} onClick={onReload}>Reload draft</button>}
        {draft && !plan && <button type="button" disabled={action.isPending} onClick={() => {
          if (window.confirm("Move this draft to Outlook Deleted Items?")) action.mutate("discard");
        }}>Discard draft</button>}
      </div>
    </form>
  </>;
}
