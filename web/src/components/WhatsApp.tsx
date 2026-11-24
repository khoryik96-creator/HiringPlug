// WhatsApp: a message to a candidate or a contact, from a template or blank,
// opened in the recruiter's own WhatsApp (a wa.me chat link) and kept as a
// WhatsApp note on their timeline.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";

import { loadApplications } from "../api/applications";
import { api, unwrap } from "../api/client";
import { candidateApplicationsKey, notesKey } from "../api/keys";
import { IconButton } from "./ActionIcons";
import { ErrorMessage } from "./ErrorMessage";
import { useCan } from "./useCan";

type Subject = { kind: "candidate" | "contact"; id: string; name: string };

export const whatsAppTemplatesKey = ["whatsapp-templates"] as const;

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

function Dialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
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
    <dialog ref={dialog} className="panel whatsapp-dialog" aria-labelledby="whatsapp-title" onCancel={(e) => { e.preventDefault(); onClose(); }}>
      <div className="row spread"><h3 id="whatsapp-title">{title}</h3><button type="button" className="link" onClick={onClose}>Close</button></div>
      {children}
    </dialog>
  );
}

export function WhatsAppDialog({ subject, onClose }: { subject: Subject; onClose: () => void }) {
  const cache = useQueryClient();
  const canPipeline = useCan("pipeline:read");
  const [templateId, setTemplateId] = useState("");
  const [applicationId, setApplicationId] = useState("");
  const [edited, setEdited] = useState<string | null>(null);
  const [numberIndex, setNumberIndex] = useState(0);
  const templates = useQuery({ queryKey: whatsAppTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/whatsapp-templates")) });
  const jobs = useQuery({
    queryKey: candidateApplicationsKey(subject.id), enabled: subject.kind === "candidate" && canPipeline,
    queryFn: ({ signal }) => loadApplications("candidate", subject.id, signal),
  });
  const draft = useQuery({
    queryKey: ["whatsapp-draft", subject.kind, subject.id, templateId, applicationId],
    staleTime: Infinity, gcTime: 0,
    queryFn: async () => unwrap(await api.POST("/v1/whatsapp-drafts", { body: {
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Kuala_Lumpur",
      ...(subject.kind === "candidate" ? { candidateId: subject.id } : { contactId: subject.id }),
      ...(templateId && { templateId }), ...(applicationId && { applicationId }),
    } })),
  });
  const text = edited ?? draft.data?.text ?? "";
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
    window.open(chatLink(chosen.chatNumber, text), "_blank", "noopener,noreferrer"); // before anything async: not a blocked pop-up
    // A blank message opens the chat to type in WhatsApp itself.
    log.mutate(text.trim() ? `WhatsApp to ${chosen.number}:\n${text}` : `WhatsApp chat opened with ${chosen.number} (no message typed).`);
  };

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
          <textarea rows={7} value={text} disabled={draft.isPending} onChange={(e) => { setEdited(e.target.value); }} />
        </label>
        {(draft.data?.missing.length ?? 0) > 0 && (
          <p className="full warning small" role="status">Not filled in: {draft.data?.missing.map((m) => `[${m}]`).join(", ")}. Change them before sending.</p>
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
