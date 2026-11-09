// The client's job description, dropped on an ad: sent up, read (a PDF or
// Word file takes a moment), and its details handed to the ad form.
import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { getToken } from "../auth";
import { DropZone } from "./DropZone";

export type JdDetails = Schemas["JdDetails"];
export const JD_TYPES = ".pdf,.docx,.doc,.txt,.rtf,.odt,.md,.png,.jpg,.jpeg,.tif,.tiff,.bmp,.webp";

/** Sends a job description (multipart, so not through the typed client); without a job, for one being added. */
async function sendJd(jobId: string | undefined, file: File): Promise<Schemas["JdFile"]> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(jobId ? `/v1/jobs/${jobId}/jd-files` : "/v1/jd-files", { method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
  const json: unknown = await res.json();
  if (!res.ok) unwrap({ error: json, response: res });
  return json as Schemas["JdFile"];
}

/**
 * A drop zone for the job description; calls onRead once with what was read
 * from it. `start` is a file dropped before this was shown (on the ads panel).
 */
export function JdDrop({ jobId, start, onRead, readText = "Check it, then save." }: {
  jobId?: string;
  start?: File | null;
  onRead: (d: JdDetails, fileName: string) => void;
  /** What to say once it is read (after "Filled in from <file>."). */
  readText?: string;
}) {
  const upload = useMutation({ mutationFn: (file: File) => sendJd(jobId, file) });
  const uploaded = upload.data;
  // A PDF or Word file is read by the worker: follow it until it is done.
  const file = useQuery({
    queryKey: ["jd-file", uploaded?.id],
    queryFn: async () => unwrap(await api.GET("/v1/jd-files/{jdFileId}", { params: { path: { jdFileId: uploaded?.id ?? "" } } })),
    enabled: uploaded?.status === "reading",
    refetchInterval: (q) => (q.state.data?.status === "reading" || !q.state.data ? 1500 : false),
  });
  const latest = file.data ?? uploaded;
  const handed = useRef<string | null>(null);
  useEffect(() => {
    if (latest?.status === "read" && latest.details && handed.current !== latest.id) {
      handed.current = latest.id;
      onRead(latest.details, latest.fileName);
    }
  }, [latest, onRead]);
  const started = useRef(false);
  useEffect(() => {
    if (start && !started.current) {
      started.current = true;
      upload.mutate(start);
    }
  }, [start, upload]);

  const reading = upload.isPending || latest?.status === "reading";
  return (
    <div className="jd-drop">
      <DropZone accept={JD_TYPES} disabled={reading} label="Drop the client's JD"
        hint="PDF, Word, text or a picture" onFiles={([f]) => { if (f) upload.mutate(f); }} />
      {reading && <p className="muted small" role="status">Reading {latest?.fileName ?? "the file"}…</p>}
      {latest?.status === "read" && <p className="small" role="status">Filled in from {latest.fileName}. {readText}</p>}
      {latest?.status === "failed" && <p className="error small">{latest.fileName}: {latest.error ?? "it could not be read."}</p>}
      {upload.error && <p className="error small">{upload.error.message}</p>}
    </div>
  );
}
