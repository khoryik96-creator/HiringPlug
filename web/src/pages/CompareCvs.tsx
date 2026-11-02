import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Document, unwrap } from "../api/client";
import { downloadDocument, fetchDocumentFile } from "../api/files";
import { compareFields, compareJobs, compareSkills } from "../components/compareCvs";
import { ErrorMessage } from "../components/ErrorMessage";

const SOURCE: Record<Document["source"], string> = {
  original: "Original", jobstreet: "JobStreet", linkedin: "LinkedIn", formatted: "Formatted", other: "Other",
};
const label = (d: Document) => `${SOURCE[d.source]}: ${d.fileName}`;

function useDocument(id: string | undefined) {
  return useQuery({
    queryKey: ["document", id],
    enabled: Boolean(id),
    queryFn: async () => unwrap(await api.GET("/v1/documents/{documentId}", { params: { path: { documentId: id ?? "" } } })),
  });
}

/** Two pre-selected versions: the original against another source when there is one. */
function defaults(docs: Document[]): [string | undefined, string | undefined] {
  const original = docs.find((d) => d.source === "original") ?? docs[0];
  const other = docs.find((d) => d.id !== original?.id && d.source !== original?.source) ?? docs.find((d) => d.id !== original?.id);
  return [original?.id, other?.id];
}

/** A candidate's CV versions side by side, with what their details disagree on. */
export function CompareCvs({ candidateId }: { candidateId: string }) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const [picked, setPicked] = useState<[string | undefined, string | undefined] | null>(null);
  const list = docs.data ?? [];
  const [leftId, rightId] = picked ?? defaults(list);
  const left = useDocument(leftId);
  const right = useDocument(rightId);

  if (docs.error) return <ErrorMessage error={docs.error} />;
  if (!docs.data) return <p className="muted">Loading…</p>;
  return (
    <main>
      <a href={`#/candidates/${candidateId}`}>← Candidate</a>
      <h2>Compare CVs</h2>
      {list.length < 2 && <p className="muted">This candidate has only one CV. Upload another version to compare.</p>}
      <Differences a={left.data} b={right.data} />
      <div className="compare">
        {[leftId, rightId].map((id, side) => (
          <Pane
            key={side}
            docs={list}
            doc={(side === 0 ? left : right).data}
            selected={id}
            onSelect={(v) => { setPicked(side === 0 ? [v, rightId] : [leftId, v]); }}
          />
        ))}
      </div>
    </main>
  );
}

function Differences({ a, b }: { a: Document | undefined; b: Document | undefined }) {
  if (!a || !b) return null;
  const pa = a.parsedProfile ?? null;
  const pb = b.parsedProfile ?? null;
  if (!pa || !pb) {
    return <p className="muted">Differences appear once details have been read from both CVs (Read details on the candidate page).</p>;
  }
  const fields = compareFields(pa, pb).filter((r) => r.differs);
  const jobs = compareJobs(pa, pb);
  const skills = compareSkills(pa, pb);
  const none = fields.length + jobs.onlyA.length + jobs.onlyB.length + jobs.changed.length + skills.onlyA.length + skills.onlyB.length === 0;
  return (
    <section className="panel" aria-label="Differences">
      <h3>Differences</h3>
      {none && <p>The details read from both CVs agree.</p>}
      {fields.length > 0 && (
        <table>
          <thead><tr><th>Field</th><th>{SOURCE[a.source]}</th><th>{SOURCE[b.source]}</th></tr></thead>
          <tbody>
            {fields.map((r) => <tr key={r.label}><td>{r.label}</td><td>{r.a || "—"}</td><td>{r.b || "—"}</td></tr>)}
          </tbody>
        </table>
      )}
      {jobs.changed.map((c) => <p key={c.a} className="warning">Changed: {c.a} → {c.b}</p>)}
      {jobs.onlyA.map((j) => <p key={j}>Only in the {SOURCE[a.source].toLowerCase()} CV: {j}</p>)}
      {jobs.onlyB.map((j) => <p key={j}>Only in the {SOURCE[b.source].toLowerCase()} CV: {j}</p>)}
      {skills.onlyA.length > 0 && <p>Skills only in the {SOURCE[a.source].toLowerCase()} CV: {skills.onlyA.join(", ")}</p>}
      {skills.onlyB.length > 0 && <p>Skills only in the {SOURCE[b.source].toLowerCase()} CV: {skills.onlyB.join(", ")}</p>}
    </section>
  );
}

function Pane({ docs, doc, selected, onSelect }: {
  docs: Document[];
  doc: Document | undefined;
  selected: string | undefined;
  onSelect: (id: string) => void;
}) {
  const [view, setView] = useState<"original" | "text">("original");
  const isPdf = doc?.contentType === "application/pdf";
  return (
    <section className="panel" aria-label={doc ? label(doc) : "CV"}>
      <div className="row">
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
      {!doc ? <p className="muted">Loading…</p> : isPdf && view === "original" ? <Original doc={doc} /> : (
        <>
          {!isPdf && <p className="muted">Word files cannot be shown in the browser; this is their text.</p>}
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
    fetchDocumentFile({ id, contentType }).then(
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
