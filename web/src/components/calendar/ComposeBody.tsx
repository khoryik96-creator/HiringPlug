// The composer's body, as an Outlook meeting's: formatted text (the email
// editor's toolbar), the person's signature, and, for an interview, an email
// template filled from the candidate and the interview.
import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { emailTemplatesKey } from "../../api/keys";
import { RichEditor } from "../clientEmail/RichEditor";
import { escapeHtml } from "../clientEmail/richText";
import { emailSignatureKey } from "../outlook/OutlookComposeTools";

const SIGNATURE = /<div data-signature="true">[\s\S]*?<\/div>\s*$/;
export const hasSignature = (html: string) => SIGNATURE.test(html);
export const withoutSignature = (html: string) => html.replace(SIGNATURE, "").trim();
export const withSignature = (html: string, signature: string) =>
  `${withoutSignature(html)}<p><br></p><div data-signature="true">${signature}</div>`;

/** The body's plain text, without the signature (an interview's instructions, an appointment's notes). */
export function bodyText(html: string): string {
  const doc = new DOMParser().parseFromString(withoutSignature(html), "text/html");
  doc.querySelectorAll("br").forEach((b) => { b.replaceWith("\n"); });
  doc.querySelectorAll("p,div,li,h1,h2,h3,h4,blockquote").forEach((e) => { e.append("\n"); });
  return doc.body.textContent.replace(/\u00a0/g, " ").replace(/[ \t]+\n/g, "\n").replace(/\n{3,}/g, "\n\n").trim();
}

/** Whether a body says anything (its signature counts: it goes to Outlook too). */
export const saysSomething = (html: string) => bodyText(html.replace(/data-signature="true"/g, "")) !== "";

/** The person's signature (Settings > Your account), "" when there is none or it cannot be read. */
export function useSignature(enabled = true): string {
  const q = useQuery({ queryKey: emailSignatureKey, queryFn: async () => unwrap(await api.GET("/v1/email-signature")), enabled, retry: false, staleTime: 300_000 });
  return q.data?.bodyHtml.trim() ?? "";
}

/**
 * The message being written, and the person's signature: a new message starts
 * with it, as Outlook adds it; toggle takes it out or puts it back.
 */
export function useMessage(initialHtml: string, isNew: boolean, enabled: boolean) {
  const signature = useSignature(enabled);
  const [message, setMessage] = useState({ html: initialHtml, reset: 0 });
  const [signed, setSigned] = useState(!isNew);
  if (!signed && signature) { // the signature has arrived: added once, as the composer opens
    setSigned(true);
    setMessage((m) => ({ html: withSignature(m.html, signature), reset: m.reset + 1 }));
  }
  const toggleSignature = () => {
    setMessage((m) => ({ html: hasSignature(m.html) ? withoutSignature(m.html) : withSignature(m.html, signature), reset: m.reset + 1 }));
  };
  /** Put in a whole new message (a template), keeping the signature where there was one. */
  const replace = (html: string) => {
    setMessage((m) => ({ html: signature && hasSignature(m.html) ? withSignature(html, signature) : html, reset: m.reset + 1 }));
  };
  const write = (html: string) => { setMessage((m) => ({ ...m, html })); };
  return { message, signature, toggleSignature, replace, write };
}

/** The formatted body, with the email editor's toolbar as icons. */
export function ComposeBody({ html, resetKey, onChange }: { html: string; resetKey: number; onChange: (html: string) => void }) {
  return <div className="compose-body"><RichEditor compact iconTools html={html} resetKey={resetKey} onChange={onChange} /></div>;
}

// Salary never goes into a calendar invitation: templates that would show it are not offered.
const SALARY = /\{\{\s*candidate\.(currentSalary|expectedSalary|salaryNotes|currentBreakdown|expectedNotes)\s*\}\}|\[(Current salary|Expected salary|Salary notes|Current salary breakdown|Expected salary notes)\]/i;

/** What a template's interview placeholders say, from the composer (the interview may not exist yet). */
export type InterviewWords = { when: string; length: string; how: string; where: string };

function fillInterview(text: string, words: InterviewWords, html: boolean): string {
  const value: Record<string, string> = { when: words.when, length: words.length, how: words.how, where: words.where };
  return text.replace(/\{\{\s*interview\.(when|length|how|where)\s*\}\}|\[(Interview time|Interview place)\]/gi, (_m, key: string | undefined, label: string | undefined) => {
    const k = key ?? (label?.toLowerCase().endsWith("time") ? "when" : "where");
    const v = value[k] ?? "";
    return html ? escapeHtml(v) : v;
  });
}

/** Picking an email template: its subject and message, filled from the candidate, their job and the interview. */
export function TemplatePicker({ candidateId, applicationId, interviewId, words, onPick }: {
  candidateId: string; applicationId: string; interviewId?: string; words: InterviewWords;
  onPick: (draft: { subject: string; html: string }) => void;
}) {
  const [open, setOpen] = useState(false);
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")), enabled: open });
  const offered = (templates.data?.items ?? []).filter((t) => !SALARY.test(`${t.subject} ${t.body} ${t.bodyHtml ?? ""}`));
  const draft = useMutation({
    mutationFn: async (templateId: string) => unwrap(await api.POST("/v1/email-drafts", {
      body: { templateId, candidateId, applicationId, interviewId, timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone },
    })),
    onSuccess: (d) => {
      const html = d.bodyHtml ?? d.body.split("\n").map((l) => `<p>${escapeHtml(l) || "<br>"}</p>`).join("");
      onPick({ subject: fillInterview(d.subject, words, false), html: fillInterview(html, words, true) });
      setOpen(false);
    },
  });
  return (
    <span className="composer-menu">
      <button type="button" className="icon-action composer-icon-tool" aria-label="Template" data-tip="Template" aria-expanded={open}
        onClick={() => { setOpen(!open); }}><TemplateIcon /></button>
      {open && (
        <span className="composer-popover" role="menu" aria-label="Templates">
          {templates.isPending && <span className="muted small">Loading…</span>}
          {templates.isError && <span className="error small">Templates could not be loaded.</span>}
          {templates.isSuccess && offered.length === 0 && <span className="muted small">No templates yet (Settings &gt; Email templates).</span>}
          {offered.map((t: Schemas["EmailTemplate"]) => (
            <button key={t.id} type="button" role="menuitem" disabled={draft.isPending} onClick={() => { draft.mutate(t.id); }}>{t.name}</button>
          ))}
          {draft.isError && <span className="error small">The template could not be filled.</span>}
        </span>
      )}
    </span>
  );
}

const Svg = ({ children }: { children: ReactNode }) => (
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{children}</svg>
);
export const TemplateIcon = () => <Svg><rect x="4" y="3" width="16" height="18" rx="2" /><path d="M8 8h8M8 12h8M8 16h5" /></Svg>;
export const SignatureIcon = () => <Svg><path d="M3 17c3-1 4-8 6-8s0 7 3 7 3-4 5-4 2 3 4 3" /><path d="M3 21h18" /></Svg>;
