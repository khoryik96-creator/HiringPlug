// One CV shown in full: the PDF itself (or its text), with a version picker.
// Used where a CV is read side by side with something else.
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState, type ReactNode } from "react";

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

export function CvPane({ docs, doc, selected, onSelect, title, highlight = "", unified = false, actions }: {
  unified?: boolean;
  actions?: ReactNode;
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
  // A PDF or a picture shows itself; a Word, RTF or ODT file shows its PDF copy once made.
  const isPdf = doc?.contentType === "application/pdf" || isImage(doc) || doc?.pdfStatus === "ready";
  return (
    <section className="panel" aria-label={doc ? label(doc) : "CV"}>
      <div className="row">
        {title && <h3 className="cv-pane-title">{title}</h3>}
        {unified && doc && <strong className="cv-selected-name">{doc.fileName}</strong>}
        {!unified && <select aria-label="CV version" value={selected ?? ""} onChange={(e) => { onSelect(e.target.value); }}>
          {docs.map((d) => <option key={d.id} value={d.id}>{label(d)}</option>)}
        </select>}
        {isPdf && (
          <button type="button" className="link" onClick={() => { setView(view === "original" ? "text" : "original"); }}>
            {view === "original" ? "Show text" : "Show original"}
          </button>
        )}
        {!unified && doc && <button type="button" className="link" onClick={() => { void downloadDocument(doc); }}>Download</button>}
        {actions}
      </div>
      {doc && marking && view === "text" && <Marked key={doc.id} doc={doc} boolean={marking} onClear={() => { setMarking(""); }} />}
      {!doc ? <p className="muted">Loading…</p> : isPdf && view === "original" ? <Original key={doc.id} doc={doc} /> : marking ? null : (
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
    (contentType === "application/pdf" || contentType.startsWith("image/") ? fetchDocumentFile({ id, contentType }) : fetchDocumentPdf(id)).then(
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
  if (!url) return <p className="muted">Loading…</p>;
  return isImage(doc)
    ? <div className="cv-picture"><img src={url} alt={doc.fileName} /></div>
    : <iframe className="cv-frame" title={doc.fileName} src={url} />;
}

/** A picture of a CV (read with OCR): shown as it is. */
const isImage = (doc: Document | undefined) => Boolean(doc?.contentType.startsWith("image/"));

/** The CV's text with the search's words marked: how many places, each word to jump to. */
function Marked({ doc, boolean, onClear }: { doc: Document; boolean: string; onClear: () => void }) {
  const hl = useQuery({
    queryKey: ["document-highlights", doc.id, boolean],
    queryFn: async () => unwrap(await api.GET("/v1/documents/{documentId}/highlights", { params: { path: { documentId: doc.id }, query: { boolean } } })),
  });
  const marks = useRef<HTMLElement[]>([]);
  const [at, setAt] = useState(-1); // the place last jumped to
  const parts = hl.data?.parts ?? [];
  const found = markedWords(parts);
  const go = (i: number) => {
    const el = marks.current[i];
    if (!el) return;
    setAt(i);
    if (typeof el.scrollIntoView === "function") el.scrollIntoView({ behavior: "smooth", block: "center" });
  };
  // A word's next place after the one shown (round to its first).
  const jump = (word: string) => {
    const places = found.order.flatMap((w, i) => (w === word ? [i] : []));
    go(places.find((i) => i > at) ?? places[0] ?? -1);
  };
  return (
    <>
      <div className="marking" role="status">
        <span>
          {hl.data ? (hl.data.matches === 0 ? "None of the words of" : `${String(hl.data.matches)} place${hl.data.matches === 1 ? "" : "s"} for`) : "Marking"}
          {" "}<code>{boolean}</code>
        </span>
        {found.words.map((w) => (
          <button key={w.key} type="button" className="kw" title={`Go to the next "${w.shown}"`} onClick={() => { jump(w.key); }}>
            {w.shown} <i>{String(w.count)}</i>
          </button>
        ))}
        {found.order.length > 0 && (
          <button type="button" className="link" onClick={() => { go((at + 1) % found.order.length); }}>
            {at < 0 ? "Go to the first" : `Next place (${String(at + 1)} of ${String(found.order.length)})`}
          </button>
        )}
        <button type="button" className="link" onClick={onClear}>Stop marking</button>
      </div>
      <ErrorMessage error={hl.error} />
      <pre className="extracted">
        {hl.data ? parts.map((p, i) => {
          if (!p.match) return <span key={i}>{p.text}</span>;
          const n = found.placeOf[i] ?? -1;
          return <mark key={i} className={n === at ? "cur" : undefined} ref={(el) => { if (el) marks.current[n] = el; }}>{p.text}</mark>;
        }) : doc.extractedText}
      </pre>
    </>
  );
}

/**
 * The marked words of a text: each word once (as first written, with how many
 * places), the word at each place in order, and each marked part's place.
 */
export function markedWords(parts: { text: string; match: boolean }[]) {
  const words = new Map<string, { key: string; shown: string; count: number }>();
  const order: string[] = [];
  const placeOf: Record<number, number> = {};
  parts.forEach((p, i) => {
    if (!p.match) return;
    const key = p.text.trim().toLowerCase();
    const word = words.get(key) ?? { key, shown: p.text.trim(), count: 0 };
    words.set(key, { ...word, count: word.count + 1 });
    placeOf[i] = order.length;
    order.push(key);
  });
  return { words: [...words.values()], order, placeOf };
}
