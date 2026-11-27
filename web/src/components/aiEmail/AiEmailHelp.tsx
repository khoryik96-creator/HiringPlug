// "✦ AI" in an email window: write it from a short ask, improve the draft
// (grammar, rewrite, friendlier, more formal, shorter), or reply to the email
// they sent. What the AI writes is shown first; "Use this" puts it in the
// email. Names, contact details, pay, links and fields never reach the AI
// (mask.ts). Shown only to those who may use the assistant, when it is on.
import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { assistantKey } from "../../api/keys";
import { ErrorMessage } from "../ErrorMessage";
import { type Held, maskHtml, maskText, type Names, unmask, unmaskSubject } from "./mask";

type Mode = Schemas["AssistantEmailMode"];
type Style = Schemas["AssistantEmailStyle"];

/** How long to wait for the AI before saying it is taking too long. */
export const AI_WAIT_MS = 3 * 60 * 1000;

const STYLES: [Style, string][] = [
  ["grammar", "Check grammar"], ["rewrite", "Rewrite"], ["friendlier", "Friendlier"], ["formal", "More formal"], ["shorter", "Shorter"],
];

export function AiEmailHelp({ recipient, subject, html, applicationId, names, onApply }: {
  recipient: Schemas["AssistantEmailRecipient"];
  subject: string;
  /** The email as it is now (the editor's HTML). */
  html: string;
  /** About a candidate's application: the AI is told the job and client. */
  applicationId?: string;
  names: Names;
  /** Puts what the AI wrote in the email (subject null: keep it). */
  onApply: (subject: string | null, html: string) => void;
}) {
  const status = useQuery({
    queryKey: assistantKey,
    queryFn: async () => unwrap(await api.GET("/v1/assistant")),
    staleTime: 60_000,
    retry: false,
  });
  const [open, setOpen] = useState(false);
  // The sender's name (for ⟦ME⟧), once the panel is opened.
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")), enabled: open });
  const [mode, setMode] = useState<Mode>(html.replace(/<[^>]*>/g, "").trim() ? "improve" : "write");
  const [ask, setAsk] = useState("");
  const [theirs, setTheirs] = useState("");
  // What each placeholder stands for, for the email being written now.
  const [held, setHeld] = useState<Held>(() => new Map());
  const myName = (me.data as { user?: { name?: string } } | undefined)?.user?.name;
  const who: Names = { ...names, me: names.me ?? myName };
  // The sender's name is held back too: nothing goes before it is known.
  const meKnown = names.me !== undefined || myName !== undefined || me.isError;
  const start = useMutation({
    mutationFn: async (style?: Style) => {
      const hold: Held = new Map();
      const draft = maskHtml(html, hold, who);
      const body: Schemas["AssistantEmailRequest"] = { mode, recipient };
      if (style) body.style = style;
      if (ask.trim()) body.instruction = maskText(ask, hold, who);
      if (subject.trim()) body.subject = maskText(subject, hold, who);
      if (draft && mode !== "write") body.draft = draft;
      if (mode === "reply") body.theirMessage = maskText(theirs, hold, who);
      if (applicationId) body.applicationId = applicationId;
      setHeld(hold);
      return unwrap(await api.POST("/v1/assistant/email-drafts", { body }));
    },
  });
  const draftId = start.data?.id;
  // A draft the AI did not write in time (its worker is down, say): asking stops.
  const [late, setLate] = useState<string | null>(null);
  useEffect(() => {
    if (!draftId) return;
    const t = window.setTimeout(() => { setLate(draftId); }, AI_WAIT_MS);
    return () => { window.clearTimeout(t); };
  }, [draftId]);
  const written = useQuery({
    queryKey: [...assistantKey, "email", draftId],
    enabled: !!draftId,
    queryFn: async () => unwrap(await api.GET("/v1/assistant/email-drafts/{draftId}", { params: { path: { draftId: draftId ?? "" } } })),
    refetchInterval: (q) => (q.state.data?.status === "done" || q.state.data?.status === "failed" || late === draftId ? false : 1500),
  });
  const tooLong = !!draftId && late === draftId && written.data?.status !== "done" && written.data?.status !== "failed";

  if (!status.data?.enabled) return null;
  const result = written.data?.status === "done" ? written.data : null;
  const busy = !meKnown || start.isPending || (!tooLong && ((!!draftId && !written.data) || written.data?.status === "queued"));
  const shownSubject = result?.subject ? unmaskSubject(result.subject, held, who) : null;
  const shownHtml = result?.body ? unmask(result.body, held, who) : "";
  const draftEmpty = !html.replace(/<[^>]*>/g, "").trim();

  return (
    <div className="ai-email">
      <button type="button" className="pill-btn ai-email-toggle" aria-expanded={open} onClick={() => { setOpen(!open); }}>✦ AI</button>
      {open && (
        <section className="ai-email-panel" aria-label="AI help with this email">
          <div className="segmented" role="group" aria-label="What the AI should do">
            {([["write", "Write it for me"], ["improve", "Improve my draft"], ["reply", "Reply to their email"]] as [Mode, string][]).map(([m, label]) => (
              <button key={m} type="button" className={mode === m ? "on" : undefined} aria-pressed={mode === m} onClick={() => { setMode(m); setAsk(""); start.reset(); }}>{label}</button>
            ))}
          </div>
          {mode === "reply" && (
            <label className="field">Their email
              <textarea rows={5} value={theirs} placeholder="Paste the email they sent" onChange={(e) => { setTheirs(e.target.value); }} />
            </label>
          )}
          {mode !== "improve" ? (
            <label className="field">{mode === "write" ? "What should it say?" : "How to answer (optional)"}
              <textarea rows={2} value={ask} placeholder={mode === "write" ? "e.g. invite her to a second interview on Thursday afternoon" : "e.g. yes to 3pm, and ask for her notice period"}
                onChange={(e) => { setAsk(e.target.value); }} />
            </label>
          ) : (
            <label className="field">Anything to keep in mind (optional)
              <input value={ask} placeholder="e.g. keep it under 100 words" onChange={(e) => { setAsk(e.target.value); }} />
            </label>
          )}
          <div className="row">
            {mode === "improve"
              ? STYLES.map(([style, label]) => (
                <button key={style} type="button" className="pill-btn" disabled={busy || draftEmpty} onClick={() => { start.mutate(style); }}>{label}</button>
              ))
              : (
                <button type="button" className="pill-btn primary" disabled={busy || (mode === "write" ? !ask.trim() : !theirs.trim())}
                  onClick={() => { start.mutate(undefined); }}>{mode === "write" ? "Write" : "Draft the reply"}</button>
              )}
            {mode === "improve" && draftEmpty && <span className="muted small">Write a draft first.</span>}
            {busy && meKnown && <span className="muted small" role="status">The AI is writing…</span>}
          </div>
          <p className="muted small">Names, contact details, pay, links and fields are kept from the AI and put back after.</p>
          <ErrorMessage error={start.error ?? written.error} />
          {written.data?.status === "failed" && <p className="warning small" role="alert">The AI could not write it: {written.data.error}</p>}
          {tooLong && (
            <p className="warning small" role="alert">
              The AI is taking too long (it may be down): try again in a few minutes, or ask whoever runs HyPlug to check the AI worker.
            </p>
          )}
          {result && (
            <section className="ai-email-result" aria-label="What the AI wrote">
              {shownSubject && <p className="small"><strong>Subject:</strong> {shownSubject}</p>}
              <div className="ai-email-body" dangerouslySetInnerHTML={{ __html: shownHtml }} />
              <div className="row">
                <button type="button" className="pill-btn primary" onClick={() => { onApply(shownSubject, shownHtml); start.reset(); setOpen(false); }}>Use this</button>
                <button type="button" className="link small" onClick={() => { start.reset(); }}>Discard</button>
              </div>
            </section>
          )}
        </section>
      )}
    </div>
  );
}
