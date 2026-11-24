// WhatsApp: a message to a candidate or a contact, from a template or blank,
// opened in the recruiter's own WhatsApp (a wa.me chat link) and kept as a
// WhatsApp note on their timeline. Their details can be put in at the cursor,
// the message saved as a template, and templates managed from the window.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";

import { loadApplications } from "../api/applications";
import { api, unwrap } from "../api/client";
import { candidateApplicationsKey, notesKey, whatsAppDraftsKey } from "../api/keys";
import { IconButton } from "./ActionIcons";
import { ErrorMessage } from "./ErrorMessage";
import { useCan } from "./useCan";
import { useWhatsAppTemplates } from "./useWhatsAppTemplates";
import { WhatsAppInsertField } from "./WhatsAppInsertField";
import { WhatsAppSaveTemplate } from "./WhatsAppSaveTemplate";
import { WhatsAppTemplateManager } from "./WhatsAppTemplateManager";
import { templateKeys, unfilledKeys, useInsertAtCursor } from "./whatsAppText";

type Subject = { kind: "candidate" | "contact"; id: string; name: string };

/** The chat link that opens WhatsApp (app or web) with the message typed in. */
export const chatLink = (chatNumber: string, text: string) =>
  `https://wa.me/${chatNumber}${text ? `?text=${encodeURIComponent(text)}` : ""}`;

export function WhatsAppIcon() {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4.5 19.5l1.1-3.6A8 8 0 1 1 8.3 18.6z" />
      <path d="M9 9.2c0 2.6 2.2 5 4.9 5.6l1.2-1.2-1.7-.8-.7.7c-.9-.4-1.7-1.2-2.1-2.1l.7-.7-.8-1.7-1.2 1.2z" fill="currentColor" strokeWidth="0.6" />
    </svg>
  );
}

/** A WhatsApp button for a candidate or contact: opens the message dialog. */
export function WhatsAppButton({ subject, hasNumber }: { subject: Subject; hasNumber: boolean }) {
  const canSend = useCan("emails:send");
  const canNote = useCan("notes:create");
  const [open, setOpen] = useState(false);
  if (!canSend || !canNote) return null;
  // Always there, so it can be found: with no number it says what it needs.
  return (
    <>
      <IconButton label="WhatsApp" tip={hasNumber ? `WhatsApp ${subject.name}` : "Add their phone number, with the country code (+60...), to WhatsApp them"}
        icon={<WhatsAppIcon />} className="whatsapp-button" disabled={!hasNumber} onClick={() => { setOpen(true); }} />
      {open && <WhatsAppDialog subject={subject} onClose={() => { setOpen(false); }} />}
    </>
  );
}

/** The window; Escape calls onEscape (by default, closing it). */
function Dialog({ title, onClose, onEscape = onClose, children }: { title: string; onClose: () => void; onEscape?: () => void; children: ReactNode }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useLayoutEffect(() => {
    const node = dialog.current;
    const previous = document.activeElement;
    node?.showModal();
    return () => {
      node?.close();
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <dialog ref={dialog} className="panel whatsapp-dialog" aria-labelledby="whatsapp-title" onCancel={(e) => { e.preventDefault(); onEscape(); }}>
      <div className="row spread"><h3 id="whatsapp-title">{title}</h3><button type="button" className="link" onClick={onClose}>Close</button></div>
      {children}
    </dialog>
  );
}

export function WhatsAppDialog({ subject, onClose }: { subject: Subject; onClose: () => void }) {
  const cache = useQueryClient();
  const canPipeline = useCan("pipeline:read");
  const canCreate = useCan("emails.templates:create");
  const canManage = useCan("emails.templates:manage");
  const [view, setView] = useState<"message" | "save" | "templates">("message");
  const [saved, setSaved] = useState("");
  const box = useRef<HTMLTextAreaElement>(null);
  const pane = useRef<HTMLDivElement>(null);
  const saveButton = useRef<HTMLButtonElement>(null);
  const templatesButton = useRef<HTMLButtonElement>(null);
  const openedFrom = useRef<"save" | "templates" | null>(null);
  const [templateId, setTemplateId] = useState("");
  const [applicationId, setApplicationId] = useState("");
  const [edited, setEdited] = useState<string | null>(null);
  const [numberIndex, setNumberIndex] = useState(0);
  const templates = useWhatsAppTemplates();
  const jobs = useQuery({
    queryKey: candidateApplicationsKey(subject.id), enabled: subject.kind === "candidate" && canPipeline,
    queryFn: ({ signal }) => loadApplications("candidate", subject.id, signal),
  });
  const draft = useQuery({
    queryKey: [...whatsAppDraftsKey, subject.kind, subject.id, templateId, applicationId],
    staleTime: Infinity, gcTime: 0,
    queryFn: async () => unwrap(await api.POST("/v1/whatsapp-drafts", { body: {
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Kuala_Lumpur",
      ...(subject.kind === "candidate" ? { candidateId: subject.id } : { contactId: subject.id }),
      ...(templateId && { templateId }), ...(applicationId && { applicationId }),
    } })),
  });
  const text = edited ?? draft.data?.text ?? "";
  const source = templates.data?.items.find((t) => t.id === templateId);
  // A template deleted (here or by someone else) sets the dropdown back to
  // Blank message once the list loads without it; the message stays as it is.
  if (templateId && templates.data && !source) {
    setTemplateId("");
    setEdited(edited ?? draft.data?.text ?? null);
  }
  const fields = draft.data?.fields ?? [];
  // What the message itself still has unfilled ("[job.title]"), however it came to be.
  const missing = unfilledKeys(text, [...fields.map((f) => f.key), ...(draft.data?.missing ?? [])]);
  // A field's value goes in where the cursor is; the message then counts as typed.
  const insert = useInsertAtCursor(box, text, setEdited);
  const numbers = draft.data?.numbers ?? [];
  const chosen = numbers.length > 0 ? numbers[Math.min(numberIndex, numbers.length - 1)] : undefined;
  const log = useMutation({
    mutationFn: async (body: string) => unwrap(await api.POST("/v1/notes", { body: { subjectType: subject.kind, subjectId: subject.id, kind: "whatsapp", body } })),
    onSuccess: async () => { await cache.invalidateQueries({ queryKey: notesKey(subject.kind, subject.id) }); onClose(); },
  });
  // A new template or job fills the message again.
  const choose = (apply: () => void) => { apply(); setEdited(null); };

  const send = () => {
    if (!chosen?.chatNumber) return;
    // A blank message (spaces only too) opens the chat to type in WhatsApp itself.
    window.open(chatLink(chosen.chatNumber, text.trim() ? text : ""), "_blank", "noopener,noreferrer"); // before anything async: not a blocked pop-up
    log.mutate(text.trim() ? `WhatsApp to ${chosen.number}:\n${text}` : `WhatsApp chat opened with ${chosen.number} (no message typed).`);
  };

  // Focus follows the pane shown: its name box or heading, and back at the
  // message, the button that opened it (or the message itself).
  useLayoutEffect(() => {
    if (view !== "message") {
      pane.current?.querySelector<HTMLElement>(view === "save" ? "input" : "h4")?.focus();
      return;
    }
    const from = openedFrom.current;
    openedFrom.current = null;
    if (!from) return;
    const opener = (from === "save" ? saveButton : templatesButton).current;
    (opener && !opener.disabled ? opener : box.current)?.focus();
  }, [view]);
  const open = (next: "save" | "templates") => { setSaved(""); openedFrom.current = next; setView(next); };
  const back = () => { setView("message"); };
  if (view !== "message") {
    // Escape here goes back to the message rather than losing it.
    return (
      <Dialog title={`WhatsApp ${subject.name}`} onClose={onClose} onEscape={back}>
        <div ref={pane}>
          {view === "save" ? (
            <WhatsAppSaveTemplate text={text} fields={fields} sourceKeys={source ? templateKeys(source.body) : []} onCancel={back}
              // Saved after the pane was left: the confirmation still shows, without changing the pane.
              onSaved={(name) => { setSaved(name); setView((v) => (v === "save" ? "message" : v)); }} />
          ) : (
            <WhatsAppTemplateManager canCreate={canCreate} canManage={canManage} onBack={back} />
          )}
        </div>
      </Dialog>
    );
  }

  return (
    <Dialog title={`WhatsApp ${subject.name}`} onClose={onClose}>
      <form className="form-grid" onSubmit={(e) => { e.preventDefault(); send(); }}>
        <label className="field">Template
          <select value={templateId} onChange={(e) => { choose(() => { setTemplateId(e.target.value); }); }}>
            <option value="">Blank message</option>
            {templates.data?.items.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
        </label>
        {subject.kind === "candidate" && (jobs.data?.items.length ?? 0) > 0 && (
          <label className="field">Job
            <select value={applicationId} onChange={(e) => { choose(() => { setApplicationId(e.target.value); }); }}>
              <option value="">No job</option>
              {jobs.data?.items.map((a) => <option key={a.id} value={a.id}>{a.jobTitle} · {a.companyName}</option>)}
            </select>
          </label>
        )}
        {numbers.length > 1 && (
          <label className="field">Number
            <select value={numberIndex} onChange={(e) => { setNumberIndex(Number(e.target.value)); }}>
              {numbers.map((n, i) => <option key={`${n.label}-${n.number}`} value={i} disabled={!n.chatNumber}>{n.label}: {n.number}{n.chatNumber ? "" : " (not a valid number)"}</option>)}
            </select>
          </label>
        )}
        {numbers.length === 1 && chosen && <p className="field muted small">To {chosen.label.toLowerCase()} {chosen.number}</p>}
        <label className="field full">Message
          <textarea ref={box} rows={7} value={text} disabled={draft.isPending} onChange={(e) => { setEdited(e.target.value); }} />
        </label>
        <div className="row full whatsapp-tools">
          <WhatsAppInsertField fields={fields} disabled={draft.isPending} onInsert={insert} />
          {canCreate && <button ref={saveButton} type="button" className="link" disabled={!text.trim()} onClick={() => { open("save"); }}>Save as template</button>}
          {(canCreate || canManage) && <button ref={templatesButton} type="button" className="link" onClick={() => { open("templates"); }}>Templates</button>}
        </div>
        {saved && <p className="full muted small" role="status">Saved as the &ldquo;{saved}&rdquo; template.</p>}
        {missing.length > 0 && (
          <p className="full warning small" role="status">Not filled in: {missing.map((m) => `[${m}]`).join(", ")}. Change them before sending.</p>
        )}
        {draft.data && !chosen?.chatNumber && <p className="full warning small" role="status">No valid phone number: save it with its country code (e.g. +60 12 345 6789).</p>}
        <ErrorMessage error={templates.error ?? draft.error ?? log.error} />
        <div className="row full">
          <button type="submit" className="whatsapp-send" disabled={!chosen?.chatNumber || draft.isPending || log.isPending}>Open in WhatsApp</button>
          <span className="muted small">Kept as a WhatsApp note.</span>
        </div>
      </form>
    </Dialog>
  );
}
