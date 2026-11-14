// Import CVs and LinkedIn PDFs as candidates, one or many at once, and follow
// each file's progress: each becomes a candidate (or joins the one with its email).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { getToken } from "../auth";
import { candidatesKey, cvImportsKey } from "../api/keys";
import { CV_TYPES, DropZone } from "./DropZone";
import { findMatches, matchText } from "./duplicateCvs";
import { SourcePicker } from "./SourcePicker";
import { useFieldSettings } from "./useFieldSettings";

type Item = Schemas["CvImportItem"];
type Status = Schemas["CvImportItemStatus"];

export const STATUS_TEXT: Record<Status, string> = {
  reading: "Reading…",
  created: "New candidate",
  added: "Added to existing candidate",
  unread: "New candidate — fill in the details",
  failed: "Failed",
  duplicate: "Skipped: already in HiringPlug",
};
const BADGE: Record<Status, string> = { reading: "queued", created: "valid", added: "unknown", unread: "risky", failed: "invalid", duplicate: "closed" };

/** Before importing: files exactly like a CV already here (skipped), and files sharing a CV's name (asked about). */
type Review = {
  skipped: { file: File; match: Schemas["DocumentMatch"] }[];
  asks: { file: File; others: Schemas["DocumentMatch"][]; upload: boolean }[];
  clean: File[];
};

/** Sends one file to an import (multipart, so not through the typed client). */
async function sendFile(importId: string, file: File): Promise<void> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(`/v1/cv-imports/${importId}/files`, {
    method: "POST",
    body,
    headers: { Authorization: `Bearer ${getToken() ?? ""}` },
  });
  if (!res.ok) unwrap({ error: await res.json(), response: res });
}

/** foldedHistory (the dashboard): past imports load only when opened, or once an import starts here. */
export function CvImports({ foldedHistory = false }: { foldedHistory?: boolean } = {}) {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const [source, setSource] = useState("linkedin");
  const [tried, setTried] = useState(false); // Import was pressed: a missing source is outlined
  const [files, setFiles] = useState<File[]>([]);
  const [sent, setSent] = useState<number | null>(null);
  const [failures, setFailures] = useState<string[]>([]);
  const [review, setReview] = useState<Review | null>(null);
  const [historyOpen, setHistoryOpen] = useState(!foldedHistory);

  const imports = useQuery({
    queryKey: cvImportsKey,
    queryFn: async () => unwrap(await api.GET("/v1/cv-imports")).items,
    enabled: historyOpen,
    // Follow the files still being read.
    refetchInterval: (q) => (q.state.data?.some((i) => i.counts.reading > 0) ? 3000 : false),
  });

  // Step one: look for CVs already in HiringPlug; import at once when nothing needs asking.
  const check = useMutation({
    mutationFn: async () => {
      const found = await findMatches(files);
      const r: Review = { skipped: [], asks: [], clean: [] };
      files.forEach((file, i) => {
        const m = found[i];
        if (m?.sameContent) r.skipped.push({ file, match: m.sameContent });
        else if (m && m.sameName.length > 0) r.asks.push({ file, others: m.sameName, upload: false });
        else r.clean.push(file);
      });
      return r;
    },
    onSuccess: (r) => {
      if (r.asks.length === 0) {
        setReview(r.skipped.length > 0 ? r : null); // what was skipped stays shown
        start.mutate(r.clean);
      } else setReview(r);
    },
    onError: () => { start.mutate(files); }, // the check is a help, not a gate
  });
  const toImport = review ? [...review.clean, ...review.asks.filter((a) => a.upload).map((a) => a.file)] : files;

  const start = useMutation({
    mutationFn: async (chosen: File[]) => {
      setFailures([]);
      if (chosen.length === 0) return;
      const imp = unwrap(await api.POST("/v1/cv-imports", { body: { source } }));
      setHistoryOpen(true); // follow this import's files
      setSent(0);
      for (const [i, f] of chosen.entries()) {
        try {
          await sendFile(imp.id, f);
        } catch (e) {
          setFailures((fl) => [...fl, `${f.name}: ${e instanceof Error ? e.message : String(e)}`]);
        }
        setSent(i + 1);
        void queryClient.invalidateQueries({ queryKey: cvImportsKey });
      }
    },
    onSettled: () => {
      setSent(null);
      setFiles([]);
      setReview((r) => (r && r.skipped.length > 0 ? { ...r, asks: [], clean: [] } : null));
      void queryClient.invalidateQueries({ queryKey: cvImportsKey });
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
    },
  });

  return (
    <section className="panel" aria-label="CV import">
      <h3>Import profiles <span className="muted small">CVs and LinkedIn PDFs</span></h3>
      <SourcePicker value={source} onChange={setSource} label="Source of these candidates" invalid={tried && source === ""} />
      <DropZone accept={CV_TYPES} multiple disabled={start.isPending} label="Drop CVs here"
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
              {!start.isPending && (
                <button type="button" className="link" aria-label={`Remove ${f.name}`}
                  onClick={() => { setFiles(files.filter((x) => x !== f)); setReview(null); }}>×</button>
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="row">
        {review && review.asks.length > 0 ? (
          <button type="button" className="primary" disabled={start.isPending} onClick={() => { start.mutate(toImport); }}>
            Import {toImport.length === 1 ? "1 file" : `${String(toImport.length)} files`}
          </button>
        ) : (
          <button type="button" className="primary" disabled={files.length === 0 || start.isPending || check.isPending}
            onClick={() => { setTried(true); if (source !== "") check.mutate(); }}>
            {check.isPending ? "Checking for CVs already here…" : `Import ${files.length === 1 ? "1 file" : `${String(files.length)} files`}`}
          </button>
        )}
        {files.length > 0 && !start.isPending && <button type="button" className="link" onClick={() => { setFiles([]); setReview(null); }}>Clear</button>}
        {sent !== null && <span className="muted">Sent {sent} of {files.length}…</span>}
      </div>
      {review && review.skipped.length > 0 && (
        <div className="dup-ask" role="status" aria-label="Skipped files">
          <p><strong>Skipped {review.skipped.length === 1 ? "1 file" : `${String(review.skipped.length)} files`}</strong>: exactly the same file is already in HiringPlug.</p>
          <ul>{review.skipped.map((x) => <li key={x.file.name}>{x.file.name}: <a href={`#/candidates/${x.match.candidateId}`}>{matchText(x.match)}</a></li>)}</ul>
        </div>
      )}
      {review && review.asks.length > 0 && (
        <div className="dup-ask" role="group" aria-label="Files with the same name as a CV already here">
          <p>These have the same name as a CV already in HiringPlug, with other content. Tick the ones to import anyway:</p>
          {review.asks.map((x, i) => (
            <label key={x.file.name} className="check">
              <input type="checkbox" checked={x.upload}
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
