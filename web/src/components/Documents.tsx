import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Document, unwrap } from "../api/client";
import { downloadDocument } from "../api/files";
import { getToken } from "../auth";
import { CvReview } from "./CvReview";
import { ErrorMessage } from "./ErrorMessage";

const SOURCES = ["original", "jobstreet", "linkedin", "formatted", "other"] as const;

/**
 * A candidate's CVs: upload, reading status (polled while the AI worker reads
 * or parses them), the text, and the details parsed from it for review.
 */
export function Documents({ candidate, canUpload, canEdit }: { candidate: Candidate; canUpload: boolean; canEdit: boolean }) {
  const candidateId = candidate.id;
  const queryClient = useQueryClient();
  const [source, setSource] = useState<(typeof SOURCES)[number]>("original");
  const [open, setOpen] = useState<string | null>(null);
  const [reviewing, setReviewing] = useState<string | null>(null);

  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
    // Keep checking while any CV is still being read by the AI worker.
    refetchInterval: (query) =>
      query.state.data?.some((d) => d.status === "queued" || d.parseStatus === "queued") ? 2000 : false,
  });

  const parse = useMutation({
    mutationFn: async (documentId: string) =>
      unwrap(await api.POST("/v1/documents/{documentId}/parse", { params: { path: { documentId } } })),
    onSuccess: (d) => {
      void queryClient.invalidateQueries({ queryKey: ["document", d.id] });
      return queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
    },
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
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["documents", candidateId] }),
  });

  return (
    <section className="panel" aria-label="Documents">
      <h3>CVs and documents</h3>
      {canUpload && (
        <div className="row">
          <select value={source} onChange={(e) => { setSource(e.target.value as (typeof SOURCES)[number]); }}>
            {SOURCES.map((s) => (
              <option key={s}>{s}</option>
            ))}
          </select>
          <input
            type="file"
            accept=".pdf,.docx,.doc"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) upload.mutate(file);
            }}
          />
        </div>
      )}
      <ErrorMessage error={upload.error ?? parse.error ?? docs.error} />
      {(docs.data?.length ?? 0) >= 2 && <a href={`#/candidates/${candidateId}/compare`}>Compare CVs side by side</a>}
      <ul>
        {docs.data?.map((d: Document) => (
          <li key={d.id}>
            <button type="button" className="link" onClick={() => { setOpen(open === d.id ? null : d.id); }}>
              {d.fileName}
            </button>{" "}
            <span className={`badge ${d.status}`}>{d.status}</span> <span className="muted">{d.source}</span>{" "}
            <button type="button" className="link" onClick={() => { void downloadDocument(d); }}>Download</button>
            {d.parseStatus === "queued" && <span className="muted"> reading details…</span>}
            {d.parseStatus === "failed" && <span className="muted"> details could not be read</span>}
            {/* Formatted CVs are our own output: parsed for comparing, not for filling the profile. */}
            {canEdit && d.parseStatus === "parsed" && d.source !== "formatted" && (
              <button type="button" className="link" onClick={() => { setReviewing(reviewing === d.id ? null : d.id); }}>
                Review details from this CV
              </button>
            )}
            {canUpload && d.status === "extracted" && d.parseStatus !== "queued" && (
              <button type="button" className="link" disabled={parse.isPending} onClick={() => { parse.mutate(d.id); }}>
                {d.parseStatus ? "Read details again" : "Read details from this CV"}
              </button>
            )}
            {d.extractionWarnings.map((w) => (
              <p key={w} className="warning">{w}</p>
            ))}
            {open === d.id && <DocumentText id={d.id} />}
            {reviewing === d.id && <CvReview documentId={d.id} candidate={candidate} onDone={() => { setReviewing(null); }} />}
          </li>
        ))}
      </ul>
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
