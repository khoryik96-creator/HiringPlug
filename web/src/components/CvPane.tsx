// One CV shown in full: the PDF itself (or its text), with a version picker.
// Used where a CV is read side by side with something else.
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Document, unwrap } from "../api/client";
import { downloadDocument, fetchDocumentFile, fetchDocumentPdf } from "../api/files";
import { ErrorMessage } from "./ErrorMessage";

export const SOURCE: Record<Document["source"], string> = {
  original: "Original", jobstreet: "JobStreet", linkedin: "LinkedIn", formatted: "Formatted", other: "Other",
};
export const label = (d: Document) => `${SOURCE[d.source]}: ${d.fileName}`;

export function useDocument(id: string | undefined) {
  return useQuery({
    queryKey: ["document", id],
    enabled: Boolean(id),
    queryFn: async () => unwrap(await api.GET("/v1/documents/{documentId}", { params: { path: { documentId: id ?? "" } } })),
  });
}

/** The CV a recruiter most likely wants: the newest one the candidate sent (not one the app formatted). */
export function latestCv(docs: Document[]): Document | undefined {
  const theirs = docs.filter((d) => d.source !== "formatted");
  return [...(theirs.length > 0 ? theirs : docs)].sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0];
}

export function CvPane({ docs, doc, selected, onSelect, title, highlight = "" }: {
  docs: Document[];
  doc: Document | undefined;
  selected: string | undefined;
  onSelect: (id: string) => void;
  /** A heading for the pane (on the candidate's page). */
  title?: string;
  /** A boolean search whose words to mark in the text (shown as text then). */
  highlight?: string;
}) {
  const [marking, setMarking] = useState(highlight.trim());
  const [view, setView] = useState<"original" | "text">(marking ? "text" : "original");
  // A PDF shows itself; a Word file shows its PDF copy once made.
  const isPdf = doc?.contentType === "application/pdf" || doc?.pdfStatus === "ready";
  return (
    <section className="panel" aria-label={doc ? label(doc) : "CV"}>
      <div className="row">
        {title && <h3 className="cv-pane-title">{title}</h3>}
        <select aria-label="CV version" value={selected ?? ""} onChange={(e) => { onSelect(e.target.value); }}>
          {docs.map((d) => <option key={d.id} value={d.id}>{label(d)}</option>)}
        </select>
        {isPdf && (
          <button type="button" className="link" onClick={() => { setView(view === "original" ? "text" : "original"); }}>
            {view === "original" ? "Show text" : "Show original"}
          </button>
        )}
        {doc && <button type="button" className="link" onClick={() => { void downloadDocument(doc); }}>Download</button>}
      </div>
      {doc && marking && view === "text" && <Marked doc={doc} boolean={marking} onClear={() => { setMarking(""); }} />}
      {!doc ? <p className="muted">Loading…</p> : isPdf && view === "original" ? <Original doc={doc} /> : marking ? null : (
        <>
          {doc.pdfStatus === "queued" && <p className="muted small" role="status">Making a preview of the Word file… its text meanwhile:</p>}
          <pre className="extracted">{doc.extractedText ?? doc.errorMessage ?? "No text has been read from this file yet."}</pre>
        </>
      )}
    </section>
  );
}

function Original({ doc }: { doc: Document }) {
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const { id, contentType } = doc;
  useEffect(() => {
    let current: string | null = null;
    let cancelled = false;
    (contentType === "application/pdf" ? fetchDocumentFile({ id, contentType }) : fetchDocumentPdf(id)).then(
      (blob) => {
        if (cancelled) return;
        current = URL.createObjectURL(blob);
        setUrl(current);
      },
      (e: unknown) => { if (!cancelled) setError(e); },
    );
    return () => {
      cancelled = true;
      if (current) URL.revokeObjectURL(current);
    };
  }, [id, contentType]); // not on every refresh of the document's details
  if (error) return <ErrorMessage error={error} />;
  return url ? <iframe className="cv-frame" title={doc.fileName} src={url} /> : <p className="muted">Loading…</p>;
}

/** The CV's text with the search's words marked, and how many places. */
function Marked({ doc, boolean, onClear }: { doc: Document; boolean: string; onClear: () => void }) {
  const hl = useQuery({
    queryKey: ["document-highlights", doc.id, boolean],
    queryFn: async () => unwrap(await api.GET("/v1/documents/{documentId}/highlights", { params: { path: { documentId: doc.id }, query: { boolean } } })),
  });
  const first = useRef<HTMLElement | null>(null);
  return (
    <>
      <div className="marking" role="status">
        <span>
          {hl.data ? (hl.data.matches === 0 ? "None of the words of" : `${String(hl.data.matches)} place${hl.data.matches === 1 ? "" : "s"} for`) : "Marking"}
          {" "}<code>{boolean}</code>
        </span>
        {hl.data && hl.data.matches > 0 && (
          <button type="button" className="link" onClick={() => { first.current?.scrollIntoView({ behavior: "smooth", block: "center" }); }}>Go to the first</button>
        )}
        <button type="button" className="link" onClick={onClear}>Stop marking</button>
      </div>
      <ErrorMessage error={hl.error} />
      <pre className="extracted">
        {hl.data ? hl.data.parts.map((p, i) => (p.match
          ? <mark key={i} ref={(el) => { if (el && !first.current) first.current = el; }}>{p.text}</mark>
          : <span key={i}>{p.text}</span>)) : doc.extractedText}
      </pre>
    </>
  );
}
