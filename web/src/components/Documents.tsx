import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Document, type Permission, unwrap } from "../api/client";
import { downloadDocument, downloadFrom } from "../api/files";
import { getToken } from "../auth";
import { CvReview } from "./CvReview";
import { CV_TYPES, DropZone } from "./DropZone";
import { ErrorMessage } from "./ErrorMessage";
import { FormatCv } from "./FormatCv";
import { StudioCvRuns } from "./StudioCvRuns";

const SOURCES = ["original", "jobstreet", "linkedin", "formatted", "other"] as const;
type Source = (typeof SOURCES)[number];
const SOURCE_LABEL: Record<Source, string> = {
  original: "Their CV", jobstreet: "JobStreet", linkedin: "LinkedIn", formatted: "Formatted CV", other: "Other",
};
const STATUS_NOTE: Partial<Record<Document["status"], string>> = {
  queued: "Reading the text…", needs_ocr: "Scanned: no text could be read", failed: "The text could not be read",
};

const added = (iso: string) => new Date(iso).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
const KIND: Record<string, string> = {
  "application/pdf": "PDF", "application/msword": "DOC", "application/rtf": "RTF", "text/plain": "TXT",
  "application/vnd.oasis.opendocument.text": "ODT",
};
const kindOf = (d: Document) => KIND[d.contentType] ?? (d.contentType.startsWith("image/") ? "IMG" : "DOCX");


/**
 * A candidate's CVs: upload, reading status (polled while the AI worker reads
 * or parses them), the text, and the details parsed from it for review. Their
 * own CVs first, then the ones HiringPlug made.
 */
export function Documents({ candidate, canUpload, canEdit, permissions = [] }: { candidate: Candidate; canUpload: boolean; canEdit: boolean; permissions?: Permission[] }) {
  const [formatting, setFormatting] = useState(false);
  const [uploading, setUploading] = useState(false);
  const candidateId = candidate.id;
  const queryClient = useQueryClient();
  const [source, setSource] = useState<Source>("original");
  const [open, setOpen] = useState<string | null>(null);
  const [reviewing, setReviewing] = useState<string | null>(null);
  const canFormat = permissions.includes("cvformats:create");

  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
    // Keep checking while any CV is still being read by the AI worker.
    refetchInterval: (query) =>
      query.state.data?.some((d) => d.status === "queued" || d.parseStatus === "queued" || d.pdfStatus === "queued") ? 2000 : false,
  });

  const parse = useMutation({
    mutationFn: async (documentId: string) =>
      unwrap(await api.POST("/v1/documents/{documentId}/parse", { params: { path: { documentId } } })),
    onSuccess: (d) => {
      void queryClient.invalidateQueries({ queryKey: ["document", d.id] });
      return queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
    },
  });

  const remove = useMutation({
    mutationFn: async (d: Document) => { unwrap(await api.DELETE("/v1/documents/{documentId}", { params: { path: { documentId: d.id } } })); },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["documents", candidateId] }),
  });
  const canDelete = permissions.includes("documents:delete");

  const pdf = useMutation({
    mutationFn: async (d: Document) => { await downloadFrom(`/v1/documents/${d.id}/pdf`, d.fileName.replace(/\.docx$/, "") + ".pdf", "application/pdf"); },
  });

  const upload = useMutation({
    mutationFn: async (file: File) => {
      const body = new FormData();
      body.append("source", source);
      body.append("file", file);
      const res = await fetch(`/v1/candidates/${candidateId}/documents`, {
        method: "POST",
        body,
        headers: { Authorization: `Bearer ${getToken() ?? ""}` },
      });
      if (!res.ok) unwrap({ error: await res.json(), response: res });
    },
    onSuccess: async () => {
      setUploading(false);
      await queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
    },
  });

  const all = docs.data ?? [];
  const groups = [
    { title: "From the candidate", items: all.filter((d) => d.source !== "formatted") },
    { title: "Formatted", items: all.filter((d) => d.source === "formatted") },
  ].filter((g) => g.items.length > 0);

  const row = (d: Document) => {
    const note = [
      STATUS_NOTE[d.status],
      d.parseStatus === "queued" ? "Reading the details…" : d.parseStatus === "failed" ? "Details could not be read" : null,
      d.pdfStatus === "queued" ? "Making the PDF…" : null,
    ].filter(Boolean).join(" · ");
    // Formatted CVs are our own output: parsed for comparing, not for filling the profile.
    const canReview = canEdit && d.parseStatus === "parsed" && d.source !== "formatted";
    const canReread = canUpload && d.status === "extracted" && d.parseStatus !== "queued";
    return (
      <li key={d.id} className="doc-card">
        <div className="doc-line">
          <span className={`doc-icon ${kindOf(d).toLowerCase()}`} aria-hidden="true">{kindOf(d)}</span>
          <div className="doc-main">
            <button type="button" className="doc-name" aria-expanded={open === d.id} title="Show the text read from it"
              onClick={() => { setOpen(open === d.id ? null : d.id); }}>{d.fileName}</button>
            <div className="doc-meta">
              {SOURCE_LABEL[d.source]} · added {added(d.createdAt)}
              {note && <span className={d.status === "queued" || d.parseStatus === "queued" ? "doc-busy" : "doc-problem"}> · {note}</span>}
            </div>
          </div>
          <div className="doc-buttons">
            {canReview && (
              <button type="button" className="doc-btn primary-soft" onClick={() => { setReviewing(reviewing === d.id ? null : d.id); }}>
                Review details
              </button>
            )}
            <button type="button" className="doc-btn" onClick={() => { void downloadDocument(d); }}>{kindOf(d) === "DOC" || kindOf(d) === "DOCX" ? "Word" : "Download"}</button>
            {d.pdfStatus === "ready" && <button type="button" className="doc-btn" onClick={() => { pdf.mutate(d); }}>PDF</button>}
            <details className="doc-more">
              <summary aria-label={`More for ${d.fileName}`}>⋯</summary>
              <div className="doc-menu" role="menu">
                <button type="button" role="menuitem" onClick={(e) => { setOpen(open === d.id ? null : d.id); e.currentTarget.closest("details")?.removeAttribute("open"); }}>
                  {open === d.id ? "Hide the text" : "Show the text"}
                </button>
                {canReread && (
                  <button type="button" role="menuitem" disabled={parse.isPending}
                    onClick={(e) => { parse.mutate(d.id); e.currentTarget.closest("details")?.removeAttribute("open"); }}>
                    {d.parseStatus ? "Read details again" : "Read details from this CV"}
                  </button>
                )}
                {d.pdfStatus === "failed" && <p className="muted small">No PDF copy (the Word file is fine).</p>}
                {canDelete && d.source === "formatted" && (
                  <button type="button" role="menuitem" className="danger" disabled={remove.isPending}
                    onClick={(e) => {
                      e.currentTarget.closest("details")?.removeAttribute("open");
                      if (window.confirm(`Delete the formatted CV "${d.fileName}"? Its Word and PDF files go too; this cannot be undone.`)) remove.mutate(d);
                    }}>
                    Delete this formatted CV
                  </button>
                )}
              </div>
            </details>
          </div>
        </div>
        {d.extractionWarnings.map((w) => (
          <p key={w} className="warning small">{w}</p>
        ))}
        {open === d.id && <DocumentText id={d.id} />}
        {reviewing === d.id && <CvReview documentId={d.id} candidate={candidate} onDone={() => { setReviewing(null); }} />}
      </li>
    );
  };

  return (
    <section className="panel docs-panel" aria-label="Documents">
      <div className="docs-head">
        <h3>CVs and documents</h3>
        <div className="docs-actions">
          {all.length >= 2 && <a className="doc-btn" href={`#/candidates/${candidateId}/compare`}>Compare side by side</a>}
          {canFormat && !formatting && <button type="button" className="doc-btn" onClick={() => { setFormatting(true); }}>Format a CV</button>}
          {canUpload && !uploading && <button type="button" className="doc-btn primary" onClick={() => { setUploading(true); }}>Upload</button>}
          {uploading && <button type="button" className="doc-btn" onClick={() => { setUploading(false); }}>Cancel upload</button>}
        </div>
      </div>
      {uploading && (
        <div className="docs-upload">
          <fieldset className="doc-kind">
            <legend>What is it?</legend>
            {SOURCES.map((s) => (
              <label key={s} className={source === s ? "on" : undefined}>
                <input type="radio" name={`source-${candidateId}`} checked={source === s} onChange={() => { setSource(s); }} /> {SOURCE_LABEL[s]}
              </label>
            ))}
          </fieldset>
          <DropZone accept={CV_TYPES} disabled={upload.isPending} label={upload.isPending ? "Uploading…" : "Drop the file here"}
            hint="PDF, Word, text, or a photo of the CV" onFiles={(files) => { if (files[0]) upload.mutate(files[0]); }} />
        </div>
      )}
      {formatting && <FormatCv candidateId={candidateId} permissions={permissions} onDone={() => { setFormatting(false); }} />}
      {canFormat && <StudioCvRuns candidateId={candidateId} />}
      <ErrorMessage error={upload.error ?? parse.error ?? docs.error ?? pdf.error ?? remove.error} />
      {docs.data && all.length === 0 && <p className="muted">No CV yet.{canUpload ? " Upload one to read their details from it." : ""}</p>}
      {groups.map((g) => (
        <div key={g.title} className="doc-group">
          {groups.length > 1 && <h4>{g.title}</h4>}
          <ul className="doc-list">{g.items.map(row)}</ul>
        </div>
      ))}
    </section>
  );
}

function DocumentText({ id }: { id: string }) {
  const doc = useQuery({
    queryKey: ["document", id],
    queryFn: async () => unwrap(await api.GET("/v1/documents/{documentId}", { params: { path: { documentId: id } } })),
  });
  if (doc.error) return <ErrorMessage error={doc.error} />;
  return <pre className="extracted">{doc.data?.extractedText ?? doc.data?.errorMessage ?? "Loading…"}</pre>;
}
