import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Document, type Permission, type Schemas, throwFor, unwrap } from "../api/client";
import { downloadDocument } from "../api/files";
import { getToken } from "../auth";
import { CandidateCv } from "./CandidateCv";
import { latestCv } from "./CvPane";
import { CvCompare } from "./CvCompare";
import { hashParam } from "../hashQuery";
import { CvReview } from "./CvReview";
import { CV_TYPES, DropZone } from "./DropZone";
import { findMatches, matchText } from "./duplicateCvs";
import { ErrorMessage } from "./ErrorMessage";
import { fileKind } from "./fileKind";
import { FormatCv } from "./FormatCv";
import { DetailsButton, isReading, ReadProgress } from "./ReadDetails";
import { StudioProgress, useStudioRun } from "./StudioProgress";

const SOURCES = ["original", "jobstreet", "linkedin", "formatted", "other"] as const;
type Source = (typeof SOURCES)[number];
const SOURCE_LABEL: Record<Source, string> = {
  original: "Their CV", jobstreet: "JobStreet", linkedin: "LinkedIn", formatted: "Formatted CV", other: "Other",
};
const STATUS_NOTE: Partial<Record<Document["status"], string>> = {
  queued: "Reading the text…", needs_ocr: "Scanned: no text could be read", failed: "The text could not be read",
};

const added = (iso: string) => new Date(iso).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
const kindOf = (d: Document) => fileKind(d.contentType);


/**
 * A candidate's CVs on their CV & files tab: upload, reading status (polled while the AI worker reads
 * or parses them), the text, and the details parsed from it for review. Their
 * own CVs first, then the ones HiringPlug made.
 */
export function Documents({ candidate, canUpload, canEdit, permissions = [] }: { candidate: Candidate; canUpload: boolean; canEdit: boolean; permissions?: Permission[] }) {
  const [picked, setPicked] = useState<string>();
  const [formatting, setFormatting] = useState(false);
  const [uploading, setUploading] = useState(false);
  const candidateId = candidate.id;
  const queryClient = useQueryClient();
  const [source, setSource] = useState<Source>("original");
  const [reviewing, setReviewing] = useState<string | null>(null);
  // CVs seen being read here (green "ready" once done, until opened), and those
  // whose details were asked for (their review opens once read).
  const [watched, setWatched] = useState<ReadonlySet<string>>(new Set());
  const [asked, setAsked] = useState<ReadonlySet<string>>(new Set());
  const [comparing, setComparing] = useState(false);
  const canFormat = permissions.includes("cvformats:create");
  const studio = useStudioRun(candidateId, canFormat);
  const [again, setAgain] = useState(false); // asked: format again, a formatted CV being saved already

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
      // The list shows it being read at once (an earlier "read" must not open the old review).
      queryClient.setQueryData<Document[]>(["documents", candidateId], (list) => list?.map((x) => (x.id === d.id ? { ...x, status: d.status, parseStatus: d.parseStatus, updatedAt: d.updatedAt } : x)));
      setAsked(new Set([...asked, d.id]));
      setWatched(new Set([...watched, d.id]));
      setPicked(d.id);
      setReviewing(null);
      void queryClient.invalidateQueries({ queryKey: ["document", d.id] });
      return queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
    },
  });

  const remove = useMutation({
    mutationFn: async (d: Document) => { unwrap(await api.DELETE("/v1/documents/{documentId}", { params: { path: { documentId: d.id } } })); },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["documents", candidateId] }),
  });
  const canDelete = permissions.includes("documents:delete");

  // Before uploading: exactly a CV they have is skipped; one with the same name is asked about.
  const [checking, setChecking] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [asking, setAsking] = useState<{ file: File; others: Schemas["DocumentMatch"][] } | null>(null);
  const check = async (file: File) => {
    setNotice(null);
    setAsking(null);
    setChecking(true);
    try {
      const [m] = await findMatches([file], candidateId);
      if (m?.sameContent) setNotice(`Skipped: ${file.name} is exactly ${matchText(m.sameContent)}, already here.`);
      else if (m && m.sameName.length > 0) setAsking({ file, others: m.sameName });
      else upload.mutate(file);
    } catch {
      upload.mutate(file); // the check is a help, not a gate: the server refuses an exact copy anyway
    } finally {
      setChecking(false);
    }
  };
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
      if (!res.ok) await throwFor(res);
    },
    onSuccess: async () => {
      setUploading(false);
      await queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
    },
  });

  const all = docs.data ?? [];
  const reviewed = all.find((d) => d.id === reviewing);
  const selected = reviewed ?? all.find(d => d.id === picked) ?? latestCv(all);
  // Follow every CV being read; one whose details were asked for opens its review once read.
  const unseen = all.filter((d) => isReading(d) && !watched.has(d.id));
  if (unseen.length > 0) setWatched(new Set([...watched, ...unseen.map((d) => d.id)]));
  const finished = (d: Document) => watched.has(d.id) && !isReading(d);
  const forget = (id: string) => {
    if (watched.has(id)) setWatched(new Set([...watched].filter((x) => x !== id)));
    if (asked.has(id)) setAsked(new Set([...asked].filter((x) => x !== id)));
  };
  const review = (id: string) => {
    if (reviewing === id) { setReviewing(null); return; }
    setReviewing(id);
    setPicked(id);
    forget(id);
  };
  const ready = all.find((d) => asked.has(d.id) && !isReading(d));
  if (ready) {
    // Read: its review opens (not a formatted CV's, read only for comparing).
    if (ready.parseStatus === "parsed" && canEdit && ready.source !== "formatted") { forget(ready.id); setReviewing(ready.id); setPicked(ready.id); }
    // Not read: the bar stays, saying so, until dismissed.
    else setAsked(new Set([...asked].filter((x) => x !== ready.id)));
  }
  const groups = [
    { title: "From the candidate", items: all.filter((d) => d.source !== "formatted") },
    { title: "Formatted", items: all.filter((d) => d.source === "formatted") },
  ].filter((g) => g.items.length > 0);

  const actions = (d: Document) => {
    const canReread = canUpload && d.status === "extracted" && d.parseStatus !== "queued";
    // A PDF shows in the viewer, which has its own download; other files download from the menu.
    const fromMenu = d.contentType !== "application/pdf";
    const word = kindOf(d) === "DOC" || kindOf(d) === "DOCX";
    // A formatted CV (our own output) is read only for comparing: no review, read again from the menu.
    const rereadFormatted = canReread && d.source === "formatted";
    const menu = fromMenu || rereadFormatted || d.pdfStatus === "failed" || (canDelete && d.source === "formatted");
    return (
      <div className="doc-buttons">
        <DetailsButton doc={d} ready={finished(d)} reviewing={reviewing === d.id}
          canReview={canEdit} canRead={canReread} busy={parse.isPending}
          onRead={() => { parse.mutate(d.id); }} onReview={() => { review(d.id); }} />
        {menu && <details className="doc-more">
          <summary aria-label={`More for ${d.fileName}`}>⋯</summary>
          <div className="doc-menu" role="menu">
            {fromMenu && <button type="button" role="menuitem" onClick={(e) => { void downloadDocument(d); e.currentTarget.closest("details")?.removeAttribute("open"); }}>
              {word ? "Download the Word file" : "Download the file"}
            </button>}
            {rereadFormatted && <button type="button" role="menuitem" disabled={parse.isPending}
              onClick={(e) => { parse.mutate(d.id); e.currentTarget.closest("details")?.removeAttribute("open"); }}>
              Read details again (for comparing)
            </button>}
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
        </details>}
      </div>);
  };
  const oneCv = selected && !(comparing && all.length >= 2);

  return (
    <section className="panel docs-panel" aria-label="Documents">
      <div className="docs-head">
        <h3>CVs and documents</h3>
        <div className="docs-actions">
          {all.length >= 2 && <button type="button" className={comparing ? "doc-btn primary-soft" : "doc-btn"} aria-pressed={comparing}
            onClick={() => { setComparing(!comparing); }}>{comparing ? "Back to one CV" : "Compare side by side"}</button>}
          {canFormat && !formatting && (studio.running
            ? <button type="button" className="doc-btn format-running" disabled>Formatting… {studio.seconds}s</button>
            : studio.finished?.status === "done"
              ? <button type="button" className="doc-btn format-done" onClick={() => { const id = studio.finished?.documentId; if (id) setPicked(id); studio.acknowledge(); }}>Done</button>
              : <button type="button" className="doc-btn" onClick={() => {
                studio.acknowledge();
                if (all.some((d) => d.source === "formatted")) setAgain(true); else setFormatting(true);
              }}>Format a CV</button>)}
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
          <DropZone accept={CV_TYPES} disabled={upload.isPending || checking} label={upload.isPending ? "Uploading…" : checking ? "Checking…" : "Drop the file here"}
            hint="PDF, Word, text, or a photo of the CV" onFiles={(files) => { if (files[0]) void check(files[0]); }} />
          {notice && <p className="muted" role="status">{notice}</p>}
          {asking && (
            <div className="dup-ask" role="alert">
              <p>
                <strong>{asking.file.name}</strong>: this candidate already has a CV with this name, {asking.others.map(matchText).join("; ")}.
                Upload it as well?
              </p>
              <div className="row">
                <button type="button" className="doc-btn primary" onClick={() => { const f = asking.file; setAsking(null); upload.mutate(f); }}>Upload anyway</button>
                <button type="button" className="doc-btn" onClick={() => { setNotice(`${asking.file.name} was not uploaded.`); setAsking(null); }}>Don&apos;t upload</button>
              </div>
            </div>
          )}
        </div>
      )}
      {again && (
        <div className="dup-ask" role="alert">
          <p>A formatted CV is saved already{(() => { const f = all.find((d) => d.source === "formatted"); return f ? <>: <strong>{f.fileName}</strong></> : null; })()}. Format again?</p>
          <div className="row">
            <button type="button" className="doc-btn primary" onClick={() => { setAgain(false); setFormatting(true); }}>Format again</button>
            <button type="button" className="doc-btn" onClick={() => { setAgain(false); }}>Cancel</button>
          </div>
        </div>
      )}
      {formatting && <FormatCv candidateId={candidateId} permissions={permissions} onDone={() => { setFormatting(false); }} />}
      {canFormat && <StudioProgress run={studio} />}
      <ErrorMessage error={upload.error ?? parse.error ?? docs.error ?? remove.error} />
      {docs.data && all.length === 0 && <p className="muted">No CV yet.{canUpload ? " Upload one to read their details from it." : ""}</p>}
      {selected && comparing && all.length >= 2 && <CvCompare docs={all} firstId={selected.id} />}
      {oneCv && selected.extractionWarnings.map((w) => <p key={w} className="warning small">{w}</p>)}
      {oneCv && <ReadProgress doc={selected} finished={finished(selected)}
        action={selected.parseStatus === "parsed" && canEdit && selected.source !== "formatted" && reviewing !== selected.id
          ? <button type="button" className="doc-btn format-done" onClick={() => { review(selected.id); }}>Review details</button> : null}
        onDismiss={() => { forget(selected.id); }} />}
      {oneCv && <div className={reviewed ? "cv-workspace reviewing" : "cv-workspace"}>
        {/* Reviewing: the details beside the CV they came from, in place of the list of files. */}
        {reviewed ? <CvReview key={reviewed.id} documentId={reviewed.id} candidate={candidate} canMerge={permissions.includes("candidates:merge")} onDone={() => { setReviewing(null); }}
          onReread={canUpload ? () => { parse.mutate(reviewed.id); } : undefined} /> : <nav className="cv-library" aria-label="Candidate files">{groups.map(group => <div key={group.title}>
          <h4>{group.title}</h4>{group.items.map(d => <button key={d.id} type="button" aria-pressed={selected.id === d.id}
            onClick={() => { setPicked(d.id); }}><strong>{d.fileName}</strong><span>{SOURCE_LABEL[d.source]} · {added(d.createdAt)} · {STATUS_NOTE[d.status] ?? "Text read"}{d.parseStatus === "failed" ? " · Details could not be read" : d.parseStatus === "queued" ? " · Reading details…" : d.parseStatus === "parsed" ? " · Details read" : ""}{d.pdfStatus === "queued" ? " · Making PDF…" : ""}</span></button>)}
        </div>)}</nav>}
        <CandidateCv candidateId={candidateId} selectedId={selected.id} unified actions={actions(selected)} highlight={hashParam("hl")} />
      </div>}
    </section>
  );
}
