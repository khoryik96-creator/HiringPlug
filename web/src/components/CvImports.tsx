// Import CVs and LinkedIn PDFs as candidates, one or many at once, and follow
// each file's progress: each becomes a candidate (or joins the one with its email).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, throwFor, unwrap } from "../api/client";
import { getToken } from "../auth";
import { candidatesKey, cvImportsKey } from "../api/keys";
import { CV_TYPES, DropZone } from "./DropZone";
import { findMatches, matchText } from "./duplicateCvs";
import { SourceSelect } from "./SourcePicker";
import { useFieldSettings } from "./useFieldSettings";

type Item = Schemas["CvImportItem"];
type Status = Schemas["CvImportItemStatus"];

export const STATUS_TEXT: Record<Status, string> = {
  reading: "Reading…",
  created: "New candidate",
  added: "Added to existing candidate",
  unread: "New candidate — fill in the details",
  failed: "Failed",
  duplicate: "Skipped: already in HyPlug",
};
const BADGE: Record<Status, string> = { reading: "queued", created: "valid", added: "unknown", unread: "risky", failed: "invalid", duplicate: "closed" };

/** Before importing: files exactly like a CV already here (skipped), and files sharing a CV's name (asked about). */
type Review = {
  skipped: { file: File; match: Schemas["DocumentMatch"] }[];
  asks: { file: File; others: Schemas["DocumentMatch"][]; upload: boolean }[];
  clean: File[];
};
type ImportRequest = { files: File[]; source: string };

/** Sends one file to an import (multipart, so not through the typed client). */
async function sendFile(importId: string, file: File, token: string | null): Promise<void> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(`/v1/cv-imports/${importId}/files`, {
    method: "POST",
    body,
    headers: { Authorization: `Bearer ${token ?? ""}` },
  });
  if (!res.ok) await throwFor(res);
}

/**
 * foldedHistory (the dashboard): past imports load only when opened, or once
 * an import starts here. initialFiles: files already dropped elsewhere (the
 * candidates page), ready to import. initialSource: the source chosen at
 * first ("" when it must be picked: files dropped elsewhere were dropped
 * without seeing it).
 */
export function CvImports({ foldedHistory = false, initialFiles, initialSource = "linkedin", moreFiles }: {
  foldedHistory?: boolean; initialFiles?: File[]; initialSource?: string;
  /** More CVs dropped on the page while this import is open: added to it. */
  moreFiles?: { files: File[]; at: number };
} = {}) {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const [source, setSource] = useState(initialSource);
  const [tried, setTried] = useState(false); // Import was pressed: a missing source is outlined
  const [files, setFiles] = useState<File[]>(initialFiles ?? []);
  const [addedAt, setAddedAt] = useState(moreFiles?.at);
  if (moreFiles && moreFiles.at !== addedAt) { // a new drop on the page: its CVs join the import
    setAddedAt(moreFiles.at);
    setFiles((had) => [...had, ...moreFiles.files.filter((d) => !had.some((h) => h.name === d.name && h.size === d.size))]);
  }
  const [progress, setProgress] = useState<{ processed: number; total: number } | null>(null);
  const [failures, setFailures] = useState<string[]>([]);
  const [review, setReview] = useState<Review | null>(null);
  const [historyOpen, setHistoryOpen] = useState(!foldedHistory);
  const [sessionToken] = useState(getToken);

  const imports = useQuery({
    queryKey: cvImportsKey,
    queryFn: async () => unwrap(await api.GET("/v1/cv-imports")).items,
    enabled: historyOpen,
    // Follow the files still being read.
    refetchInterval: (q) => (q.state.data?.some((i) => i.counts.reading > 0) ? 3000 : false),
  });

  // Step one: look for CVs already in HyPlug; import at once when nothing needs asking.
  const check = useMutation({
    mutationFn: async (request: ImportRequest) => {
      if (getToken() !== sessionToken) throw new Error("Your sign-in changed. Open the page again before importing.");
      const found = await findMatches(request.files);
      const r: Review = { skipped: [], asks: [], clean: [] };
      request.files.forEach((file, i) => {
        const m = found[i];
        if (m?.sameContent) r.skipped.push({ file, match: m.sameContent });
        else if (m && m.sameName.length > 0) r.asks.push({ file, others: m.sameName, upload: false });
        else r.clean.push(file);
      });
      return r;
    },
    onSuccess: (r, request) => {
      if (r.asks.length === 0) {
        setReview(r.skipped.length > 0 ? r : null); // what was skipped stays shown
        start.mutate({ ...request, files: r.clean });
      } else setReview(r);
    },
    onError: (_error, request) => { start.mutate(request); }, // the upload checks for exact duplicates too
  });
  const toImport = review ? [...review.clean, ...review.asks.filter((a) => a.upload).map((a) => a.file)] : files;

  const start = useMutation({
    mutationFn: async (request: ImportRequest) => {
      setFailures([]);
      if (getToken() !== sessionToken) throw new Error("Your sign-in changed. Open the page again before importing.");
      if (request.files.length === 0) return [];
      const imp = unwrap(await api.POST("/v1/cv-imports", { body: { source: request.source } }));
      setHistoryOpen(true); // follow this import's files
      const failed: File[] = [];
      setProgress({ processed: 0, total: request.files.length });
      for (const [i, f] of request.files.entries()) {
        try {
          if (getToken() !== sessionToken) throw new Error("Your sign-in changed. Open the page again before importing.");
          await sendFile(imp.id, f, sessionToken);
        } catch (e) {
          failed.push(f);
          setFailures((fl) => [...fl, `${f.name}: ${e instanceof Error ? e.message : String(e)}`]);
        }
        setProgress({ processed: i + 1, total: request.files.length });
        void queryClient.invalidateQueries({ queryKey: cvImportsKey });
      }
      return failed;
    },
    onSuccess: (failed) => {
      setFiles(failed);
      setReview((r) => (r && r.skipped.length > 0 ? { ...r, asks: [], clean: [] } : null));
    },
    onSettled: () => {
      setProgress(null);
      void queryClient.invalidateQueries({ queryKey: cvImportsKey });
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
    },
  });
  const busy = check.isPending || start.isPending;

  return (
    <section className="panel" aria-label="CV import">
      <div className={foldedHistory ? "import-line" : undefined}>
      <h3>Import profiles {!foldedHistory && <span className="muted small">CVs and LinkedIn PDFs</span>}</h3>
      <SourceSelect value={source} onChange={setSource} invalid={tried && source === ""} disabled={busy} />
      </div>
      {/* Under the source, roomy enough to aim at. */}
      <DropZone accept={CV_TYPES} multiple disabled={busy} label="Drop CVs here"
        hint="PDF, Word, text or a photo of each CV"
        onFiles={(dropped) => {
          // Dropping more adds to the list; the same file twice is kept once.
          setReview(null);
          setFiles((had) => [...had, ...dropped.filter((d) => !had.some((h) => h.name === d.name && h.size === d.size))]);
        }} />
      {files.length > 0 && (
        <ul className="drop-files" aria-label="Files to import">
          {files.map((f) => (
            <li key={`${f.name}-${String(f.size)}`}>
              {f.name}
              {!busy && (
                <button type="button" className="link" aria-label={`Remove ${f.name}`}
                  onClick={() => { setFiles(files.filter((x) => x !== f)); setReview(null); }}>×</button>
              )}
            </li>
          ))}
        </ul>
      )}
      {(!foldedHistory || files.length > 0 || review) && <div className="row">
        {review && review.asks.length > 0 ? (
          <button type="button" className="primary" disabled={busy || source === "" || toImport.length === 0}
            onClick={() => { start.mutate({ files: toImport, source }); }}>
            Import {toImport.length === 1 ? "1 file" : `${String(toImport.length)} files`}
          </button>
        ) : (
          <button type="button" className="primary" disabled={files.length === 0 || busy}
            onClick={() => { setTried(true); if (source !== "") check.mutate({ files, source }); }}>
            {check.isPending ? "Checking for CVs already here…" : `Import ${files.length === 1 ? "1 file" : `${String(files.length)} files`}`}
          </button>
        )}
        {files.length > 0 && !busy && <button type="button" className="link" onClick={() => { setFiles([]); setReview(null); }}>Clear</button>}
        {progress && <span className="muted">Processed {progress.processed} of {progress.total}…</span>}
      </div>}
      {review && review.skipped.length > 0 && (
        <div className="dup-ask" role="status" aria-label="Skipped files">
          <p><strong>Skipped {review.skipped.length === 1 ? "1 file" : `${String(review.skipped.length)} files`}</strong>: exactly the same file is already in HyPlug.</p>
          <ul>{review.skipped.map((x) => <li key={x.file.name}>{x.file.name}: <a href={`#/candidates/${x.match.candidateId}`}>{matchText(x.match)}</a></li>)}</ul>
        </div>
      )}
      {review && review.asks.length > 0 && (
        <div className="dup-ask" role="group" aria-label="Files with the same name as a CV already here">
          <p>These have the same name as a CV already in HyPlug, with other content. Tick the ones to import anyway:</p>
          {review.asks.map((x, i) => (
            <label key={x.file.name} className="check">
              <input type="checkbox" checked={x.upload} disabled={busy}
                onChange={(e) => { setReview({ ...review, asks: review.asks.map((y, j) => (j === i ? { ...y, upload: e.target.checked } : y)) }); }} />{" "}
              <strong>{x.file.name}</strong>: like {x.others.map(matchText).join("; ")}
            </label>
          ))}
        </div>
      )}
      {start.error && <p className="error">{start.error.message}</p>}
      {failures.map((f) => <p key={f} className="error">{f}</p>)}

      {foldedHistory && (
        <button type="button" className="link small" aria-expanded={historyOpen} onClick={() => { setHistoryOpen(!historyOpen); }}>
          {historyOpen ? "Hide recent imports" : "Show recent imports"}
        </button>
      )}
      {historyOpen && imports.data?.map((imp) => (
        <div key={imp.id} aria-label={`Import of ${new Date(imp.createdAt).toLocaleString()}`}>
          <h4>
            {new Date(imp.createdAt).toLocaleString()} · {fs.label("candidate_source", imp.source)}:{" "}
            {summary(imp.counts)}
          </h4>
          <table>
            <thead><tr><th>File</th><th>Result</th><th>Candidate</th></tr></thead>
            <tbody>{imp.items.map((it) => <ItemRow key={it.id} item={it} />)}</tbody>
          </table>
        </div>
      ))}
      {historyOpen && imports.data?.length === 0 && <p className="muted">No imports yet.</p>}
    </section>
  );
}

function summary(c: Schemas["CvImportCounts"]): string {
  const parts: [number, string][] = [
    [c.reading, "reading"], [c.created, "new"], [c.added, "added to existing"], [c.unread, "to fill in"], [c.failed, "failed"],
  ];
  return parts.filter(([n]) => n > 0).map(([n, t]) => `${String(n)} ${t}`).join(" · ") || "no files";
}

function ItemRow({ item }: { item: Item }) {
  return (
    <tr>
      <td>{item.fileName}</td>
      <td>
        <span className={`badge ${BADGE[item.status]}`}>{STATUS_TEXT[item.status]}</span>
        {item.error && <small className="muted"> {item.error}</small>}
      </td>
      <td>
        {item.candidateId
          ? <a href={`#/candidates/${item.candidateId}`}>{item.candidateName ?? "Candidate"}</a>
          : <span className="muted">—</span>}
      </td>
    </tr>
  );
}
