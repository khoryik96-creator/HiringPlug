// Import CVs and LinkedIn PDFs as candidates, one or many at once, and follow
// each file's progress: each becomes a candidate (or joins the one with its email).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { getToken } from "../auth";
import { candidatesKey, cvImportsKey } from "../api/keys";
import { DropZone } from "./DropZone";
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
};
const BADGE: Record<Status, string> = { reading: "queued", created: "valid", added: "unknown", unread: "risky", failed: "invalid" };

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

export function CvImports() {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const [source, setSource] = useState("linkedin");
  const [files, setFiles] = useState<File[]>([]);
  const [sent, setSent] = useState<number | null>(null);
  const [failures, setFailures] = useState<string[]>([]);

  const imports = useQuery({
    queryKey: cvImportsKey,
    queryFn: async () => unwrap(await api.GET("/v1/cv-imports")).items,
    // Follow the files still being read.
    refetchInterval: (q) => (q.state.data?.some((i) => i.counts.reading > 0) ? 3000 : false),
  });

  const start = useMutation({
    mutationFn: async () => {
      setFailures([]);
      const imp = unwrap(await api.POST("/v1/cv-imports", { body: { source } }));
      setSent(0);
      for (const [i, f] of files.entries()) {
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
      void queryClient.invalidateQueries({ queryKey: cvImportsKey });
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
    },
  });

  return (
    <section className="panel" aria-label="CV import">
      <h3>CVs and LinkedIn PDFs</h3>
      <p className="muted">
        Each file becomes a candidate, owned by you, filled in from the CV (name, email, phone, LinkedIn link, current job, work
        history, education, skills, industry). A file whose email belongs to a candidate is added to that candidate instead.
        On LinkedIn: open the profile, <em>More → Save to PDF</em>.
      </p>
      <SourcePicker value={source} onChange={setSource} label="Source of these candidates" />
      <DropZone accept=".pdf,.docx,.doc" multiple disabled={start.isPending} label="Drop CVs here"
        hint="PDF or Word, as many as you like"
        onFiles={(dropped) => {
          // Dropping more adds to the list; the same file twice is kept once.
          setFiles((had) => [...had, ...dropped.filter((d) => !had.some((h) => h.name === d.name && h.size === d.size))]);
        }} />
      {files.length > 0 && (
        <ul className="drop-files" aria-label="Files to import">
          {files.map((f) => (
            <li key={`${f.name}-${String(f.size)}`}>
              {f.name}
              {!start.isPending && (
                <button type="button" className="link" aria-label={`Remove ${f.name}`}
                  onClick={() => { setFiles(files.filter((x) => x !== f)); }}>×</button>
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="row">
        <button type="button" className="primary" disabled={files.length === 0 || source === "" || start.isPending} onClick={() => { start.mutate(); }}>
          Import {files.length === 1 ? "1 file" : `${String(files.length)} files`}
        </button>
        {files.length > 0 && !start.isPending && <button type="button" className="link" onClick={() => { setFiles([]); }}>Clear</button>}
        {sent !== null && <span className="muted">Sent {sent} of {files.length}…</span>}
      </div>
      {start.error && <p className="error">{start.error.message}</p>}
      {failures.map((f) => <p key={f} className="error">{f}</p>)}

      {imports.data?.map((imp) => (
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
      {imports.data?.length === 0 && <p className="muted">No imports yet.</p>}
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
