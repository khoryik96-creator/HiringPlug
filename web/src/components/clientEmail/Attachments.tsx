// What goes with the email: the candidate's CVs from the ATS (a formatted CV
// as PDF too, once made) and files from the computer.
import { useMutation, useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import { api, type Schemas, throwFor, unwrap } from "../../api/client";
import { getToken } from "../../auth";
import { PaperclipIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";

export type CvChoice = Schemas["EmailCvAttachment"];
type Upload = Schemas["EmailAttachment"];

const SOURCE: Record<string, string> = { original: "Original", jobstreet: "JobStreet", linkedin: "LinkedIn", formatted: "Formatted", other: "Other" };
const mb = (bytes: number) => bytes < 1048576 ? `${String(Math.max(1, Math.round(bytes / 1024)))} KB` : `${(bytes / 1048576).toFixed(1)} MB`;

export function Attachments({ candidateId, canSeeCvs, cvs, onCvs, uploads, onUploads, others = 0, onBusy }: {
  candidateId: string;
  onBusy?: (busy:boolean)=>void;
  /** Files chosen elsewhere (each candidate's profile, on an email about several). */
  others?: number;
  canSeeCvs: boolean;
  cvs: CvChoice[];
  onCvs: (v: CvChoice[]) => void;
  uploads: Upload[];
  onUploads: (v: Upload[]) => void;
}) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    enabled: canSeeCvs,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const upload = useMutation({
    onMutate:()=>{onBusy?.(true);},
    onSettled:()=>{onBusy?.(false);},
    mutationFn: async (files: File[]) => {
      const out: Upload[] = [];
      for (const f of files) {
        const body = new FormData();
        body.append("file", f);
        const res = await fetch("/v1/email-attachments", { method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
        if (!res.ok) await throwFor(res);
        out.push((await res.json()) as Upload);
      }
      return out;
    },
    onSuccess: (added) => { onUploads([...uploads, ...added]); },
  });
  const has = (id: string, asPdf: boolean) => cvs.some((c) => c.documentId === id && Boolean(c.asPdf) === asPdf);
  const toggle = (id: string, asPdf: boolean) => {
    onCvs(has(id, asPdf) ? cvs.filter((c) => !(c.documentId === id && Boolean(c.asPdf) === asPdf)) : [...cvs, { documentId: id, asPdf }]);
  };
  const count = cvs.length + uploads.length;

  const input = useRef<HTMLInputElement>(null);
  const ats = canSeeCvs ? docs.data ?? [] : [];

  return (
    <section className="email-attachments" aria-label="Attachments">
      <div className="email-attachments-head">
        <span className="email-attachments-title">Attachments</span>
        <span className="muted small">{count + others} of 10 · 20 MB in all</span>
        <button type="button" className="secondary small" disabled={upload.isPending} onClick={() => { input.current?.click(); }}>
          <PaperclipIcon /> {upload.isPending ? "Uploading…" : "Attach files"}
        </button>
        <input ref={input} type="file" multiple hidden aria-label="Attach files" onChange={(e) => {
          const files = [...(e.target.files ?? [])];
          if (files.length > 0) upload.mutate(files);
          e.target.value = "";
        }} />
      </div>
      {canSeeCvs && (
        <div className="email-attachments-cvs">
          <span className="muted small">From the ATS</span>
          {docs.data?.length === 0 && <span className="muted small">This candidate has no CV yet.</span>}
          {ats.map((d) => (
            <span key={d.id} className="file-chip-group">
              <label className={`file-chip choice${has(d.id, false) ? " on" : ""}`}>
                <input type="checkbox" checked={has(d.id, false)} onChange={() => { toggle(d.id, false); }} />
                {SOURCE[d.source] ?? d.source}: {d.fileName} <span className="muted">({mb(d.sizeBytes)})</span>
              </label>
              {d.source === "formatted" && d.pdfStatus === "ready" && (
                <label className={`file-chip choice${has(d.id, true) ? " on" : ""}`}>
                  <input type="checkbox" checked={has(d.id, true)} onChange={() => { toggle(d.id, true); }} /> as PDF
                </label>
              )}
            </span>
          ))}
        </div>
      )}
      {uploads.length > 0 && (
        <ul className="file-chips" aria-label="Files attached">
          {uploads.map((u) => (
            <li key={u.id} className="file-chip">
              <PaperclipIcon /> {u.fileName} <span className="muted">({mb(u.sizeBytes)})</span>
              <button type="button" className="file-chip-remove" aria-label={`Remove ${u.fileName}`}
                onClick={() => { onUploads(uploads.filter((x) => x.id !== u.id)); }}>×</button>
            </li>
          ))}
        </ul>
      )}
      <ErrorMessage error={upload.error} />
    </section>
  );
}
