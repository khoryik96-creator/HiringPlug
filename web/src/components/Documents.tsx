import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Document, unwrap } from "../api/client";
import { getToken } from "../auth";
import { ErrorMessage } from "./ErrorMessage";

const SOURCES = ["original", "jobstreet", "linkedin", "formatted", "other"] as const;

/** A candidate's CVs: upload, extraction status (polled while queued) and text. */
export function Documents({ candidateId, canUpload }: { candidateId: string; canUpload: boolean }) {
  const queryClient = useQueryClient();
  const [source, setSource] = useState<(typeof SOURCES)[number]>("original");
  const [open, setOpen] = useState<string | null>(null);

  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
    // Keep checking while any CV is still being read by the AI worker.
    refetchInterval: (query) => (query.state.data?.some((d) => d.status === "queued") ? 2000 : false),
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
            accept=".pdf,.docx"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) upload.mutate(file);
            }}
          />
        </div>
      )}
      <ErrorMessage error={upload.error ?? docs.error} />
      <ul>
        {docs.data?.map((d: Document) => (
          <li key={d.id}>
            <button type="button" className="link" onClick={() => { setOpen(open === d.id ? null : d.id); }}>
              {d.fileName}
            </button>{" "}
            <span className={`badge ${d.status}`}>{d.status}</span> <span className="muted">{d.source}</span>
            {d.extractionWarnings.map((w) => (
              <p key={w} className="warning">{w}</p>
            ))}
            {open === d.id && <DocumentText id={d.id} />}
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
