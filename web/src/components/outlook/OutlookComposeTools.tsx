// HiringPlug's email tools in the Outlook Mail editor (new mail, reply,
// reply all, forward): templates (use, save, save as new, delete), a
// candidate whose details fill fields such as [Expected salary], banners,
// the person's signature, and a candidate's CV to attach.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Permission, unwrap } from "../../api/client";
import { emailTemplatesKey } from "../../api/keys";
import { getToken } from "../../auth";
import { ErrorMessage } from "../ErrorMessage";
import { plainToHtml, valuesToTokens } from "../clientEmail/richText";
import { useBanners } from "../clientEmail/useBanners";
import { atStart, ownPart } from "./outlookSignature";

export const emailSignatureKey = ["email-signature"] as const;
const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

export type ComposeContext = { candidateId?: string; contactId?: string };

/** What the editor needs: templates' fields, the chosen candidate's values, banners, the signature. */
export function useComposeData(about: ComposeContext) {
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const banners = useBanners();
  const signature = useQuery({ queryKey: emailSignatureKey, queryFn: async () => unwrap(await api.GET("/v1/email-signature")) });
  const context = useQuery({
    queryKey: ["email-context", "outlook", about.candidateId ?? "", about.contactId ?? ""],
    queryFn: async () => unwrap(await api.POST("/v1/email-previews", {
      body: { subject: "", bodyHtml: "", timeZone: timeZone(), ...(about.candidateId ? { candidateId: about.candidateId } : {}), ...(about.contactId ? { contactId: about.contactId } : {}) },
    })),
  });
  return {
    templates: templates.data?.items ?? [], fields: templates.data?.fields ?? [], values: context.data?.fieldValues,
    banners: banners.banners, bannerUrls: banners.urls, uploadBanner: banners.upload,
    signature: signature.data?.bodyHtml ?? "", signatureLoaded: signature.isSuccess, error: templates.error ?? context.error ?? banners.upload.error,
  };
}

type Data = ReturnType<typeof useComposeData>;

export function OutlookComposeTools({ data, html, subject, permissions, candidate, onCandidate, onInsert, onSubject, onSignature, onAttach, disabled }: {
  data: Data; html: string; subject: string; permissions: Permission[];
  candidate: { id: string; name: string } | null; onCandidate: (c: { id: string; name: string } | null) => void;
  onInsert: (html: string) => void; onSubject: (subject: string) => void; onSignature: () => void; onAttach: (file: File) => void;
  disabled: boolean;
}) {
  const client = useQueryClient();
  const canManage = permissions.includes("emails.templates:manage");
  const canCreate = canManage || permissions.includes("emails.templates:create");
  const [picked, setPicked] = useState("");
  const [name, setName] = useState("");
  const selected = data.templates.find((t) => t.id === picked);
  const choose = (id: string) => { setPicked(id); setName(data.templates.find((t) => t.id === id)?.name ?? ""); };
  const use = () => {
    if (!selected) return;
    if (!subject.trim()) onSubject(selected.subject);
    // A plain-text template is text: escaped, never read as markup.
    onInsert(atStart(html, selected.bodyHtml ?? plainToHtml(selected.body)));
  };
  const save = useMutation({
    mutationFn: async (asNew: boolean) => {
      // What the person wrote (not the quoted message or the signature); the candidate's details go back in as fields.
      const body = { name: name.trim(), subject: valuesToTokens(subject.replace(/^(re|fw|fwd):\s*/i, ""), data.values ?? {}), bodyHtml: ownPart(html) };
      if (asNew || !selected) return unwrap(await api.POST("/v1/email-templates", { body }));
      return unwrap(await api.PUT("/v1/email-templates/{templateId}", { params: { path: { templateId: selected.id } }, body: { ...body, version: selected.version } }));
    },
    onSuccess: async (t) => { await client.invalidateQueries({ queryKey: emailTemplatesKey }); setPicked(t.id); setName(t.name); },
  });
  const remove = useMutation({
    mutationFn: async (id: string) => { unwrap(await api.DELETE("/v1/email-templates/{templateId}", { params: { path: { templateId: id } } })); },
    onSuccess: async () => { await client.invalidateQueries({ queryKey: emailTemplatesKey }); setPicked(""); setName(""); },
  });
  return (
    <fieldset className="full outlook-compose-tools" disabled={disabled} aria-label="HiringPlug tools">
      <div className="row outlook-tools-row">
        <select aria-label="Template" value={picked} onChange={(e) => { choose(e.target.value); }}>
          <option value="">Template…</option>
          {data.templates.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>
        <button type="button" disabled={!selected} onClick={use}>Use template</button>
        {permissions.includes("candidates:read") && <CandidatePicker candidate={candidate} onChange={onCandidate} />}
        <button type="button" disabled={!data.signature} title={data.signature ? undefined : "Write your signature in Settings > Your account"}
          onClick={onSignature}>Insert signature</button>
        {candidate && permissions.includes("documents:read") && <AttachCv candidateId={candidate.id} onAttach={onAttach} />}
      </div>
      {canCreate && (
        <details className="outlook-template-save">
          <summary>Save as template</summary>
          <div className="row">
            <input aria-label="Template name" placeholder="Template name" value={name} maxLength={100} onChange={(e) => { setName(e.target.value); }} />
            {canManage && selected && <button type="button" disabled={!name.trim() || save.isPending} onClick={() => { save.mutate(false); }}>Save changes to “{selected.name}”</button>}
            <button type="button" disabled={!name.trim() || save.isPending} onClick={() => { save.mutate(true); }}>Save as new template</button>
            {canManage && selected && <button type="button" className="link danger" disabled={remove.isPending}
              onClick={() => { if (window.confirm(`Delete the template "${selected.name}"?`)) remove.mutate(selected.id); }}>Delete template</button>}
          </div>
          {save.isSuccess && <p className="small muted" role="status">Saved “{save.data.name}”: what you wrote, without the quoted message or your signature.</p>}
        </details>
      )}
      {!candidate && data.fields.length > 0 && <p className="small muted">Choose a candidate to fill fields such as [Expected salary] with their details.</p>}
      <UnfilledFields html={html} subject={subject} fields={data.fields} />
      {!permissions.includes("candidates.compensation:read") && <p className="small muted">Salary fields need a role that may see salaries.</p>}
      <ErrorMessage error={data.error ?? save.error ?? remove.error} />
    </fieldset>
  );
}

function CandidatePicker({ candidate, onChange }: { candidate: { id: string; name: string } | null; onChange: (c: { id: string; name: string } | null) => void }) {
  const [text, setText] = useState("");
  const [query, setQuery] = useState("");
  useEffect(() => { const t = setTimeout(() => { setQuery(text.trim()); }, 250); return () => { clearTimeout(t); }; }, [text]);
  const found = useQuery({
    queryKey: ["outlook-candidate-search", query], enabled: query.length > 1,
    queryFn: async () => unwrap(await api.GET("/v1/candidates", { params: { query: { q: query, limit: 8 } } })).items,
  });
  if (candidate) {
    return <span className="outlook-candidate-chip">For {candidate.name}
      <button type="button" className="link" aria-label="Choose another candidate" onClick={() => { onChange(null); }}>×</button></span>;
  }
  return (
    <span className="outlook-candidate-picker">
      <input type="search" aria-label="Candidate for fields" placeholder="Candidate for fields…" value={text} onChange={(e) => { setText(e.target.value); }} />
      {query.length > 1 && (found.data?.length ?? 0) > 0 && (
        <span className="outlook-move-menu" role="listbox" aria-label="Candidates">
          {found.data?.map((c) => <button type="button" role="option" aria-selected={false} key={c.id}
            onClick={() => { onChange({ id: c.id, name: c.fullName }); setText(""); setQuery(""); }}>{c.fullName}{c.currentTitle ? ` · ${c.currentTitle}` : ""}</button>)}
        </span>
      )}
    </span>
  );
}

function AttachCv({ candidateId, onAttach }: { candidateId: string; onAttach: (file: File) => void }) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const select = useRef<HTMLSelectElement>(null);
  const fetchFile = useMutation({
    mutationFn: async (id: string) => {
      const doc = docs.data?.find((d) => d.id === id);
      const res = await fetch(`/v1/documents/${id}/file`, { headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
      if (!res.ok || !doc) throw new Error("The CV could not be fetched.");
      return new File([await res.blob()], doc.fileName, { type: doc.contentType });
    },
    onSuccess: (file) => { onAttach(file); if (select.current) select.current.value = ""; },
  });
  if (!docs.data?.length) return null;
  return <>
    <select ref={select} aria-label="Attach CV" defaultValue="" disabled={fetchFile.isPending} onChange={(e) => { if (e.target.value) fetchFile.mutate(e.target.value); }}>
      <option value="">{fetchFile.isPending ? "Attaching…" : "Attach CV…"}</option>
      {docs.data.map((d) => <option key={d.id} value={d.id}>{d.fileName}</option>)}
    </select>
    <ErrorMessage error={fetchFile.error} />
  </>;
}

/** Fields still in brackets (no value for this candidate, or none chosen): the person fills them in or takes them out. */
export function unfilledFields(html: string, subject: string, tokens: string[]): string[] {
  const text = `${subject} ${new DOMParser().parseFromString(html, "text/html").body.textContent}`;
  return tokens.filter((t) => text.includes(t));
}

function UnfilledFields({ html, subject, fields }: { html: string; subject: string; fields: { token: string }[] }) {
  const missing = unfilledFields(ownPart(html), subject, fields.map((f) => f.token));
  if (missing.length === 0) return null;
  return <p className="warning small" role="status">Not filled in: {missing.join(", ")}. Type them or take them out before sending.</p>;
}
