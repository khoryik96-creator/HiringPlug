// The client's job descriptions kept with a job (dropped here, on an ad, or
// on the Add job form): view them in the app, download them as sent, and
// delete them for those given jobs.documents:delete (Settings › Roles).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { jobJdFilesKey } from "../api/keys";
import { formatWhen } from "../labels";
import { DropZone } from "./DropZone";
import { ErrorMessage } from "./ErrorMessage";
import { FileViewer } from "./FileViewer";
import { fileKind, fileSize } from "./fileKind";
import { JD_TYPES, sendJd } from "./JdDrop";

type JdFile = Schemas["JdFile"];

const viewable = (f: JdFile) => f.contentType === "application/pdf" || f.contentType === "text/plain" || f.contentType.startsWith("image/");
const fileUrl = (f: JdFile) => `/v1/jd-files/${f.id}/file`;

export function JdFiles({ jobId, permissions }: { jobId: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const key = jobJdFilesKey(jobId);
  const files = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/jd-files", { params: { path: { jobId } } })).items,
    // A PDF or Word file is read by the AI worker: show when it is done.
    refetchInterval: (q) => (q.state.data?.some((f) => f.status === "reading") ? 2000 : false),
  });
  const upload = useMutation({
    mutationFn: async (list: File[]) => { for (const f of list) await sendJd(jobId, f); },
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  });
  const remove = useMutation({
    mutationFn: async (f: JdFile) => { unwrap(await api.DELETE("/v1/jd-files/{jdFileId}", { params: { path: { jdFileId: f.id } } })); },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  });
  const download = useMutation({ mutationFn: (f: JdFile) => downloadFrom(fileUrl(f), f.fileName, f.contentType) });
  const [viewing, setViewing] = useState<JdFile | null>(null);
  const canAdd = permissions.includes("jobs:update");
  const canDelete = permissions.includes("jobs.documents:delete");
  const list = files.data ?? [];

  return (
    <div className="jd-files" role="group" aria-label="Job description files">
      <h4>Job description files {list.length > 0 && <span className="muted">({String(list.length)})</span>}</h4>
      <ErrorMessage error={files.error ?? upload.error ?? remove.error ?? download.error} />
      {list.length > 0 && (
        <ul className="company-doc-list">
          {list.map((f) => (
            <li key={f.id} className="doc-line">
              <span className={`doc-icon ${fileKind(f.contentType).toLowerCase()}`} aria-hidden="true">{fileKind(f.contentType)}</span>
              <div className="doc-main">
                <button type="button" className="doc-name" title={viewable(f) ? "View" : "Download"}
                  onClick={() => { if (viewable(f)) setViewing(f); else download.mutate(f); }}>{f.fileName}</button>
                <div className="doc-meta">
                  {fileSize(f.sizeBytes)} · added {formatWhen(f.createdAt)}
                  {f.status === "reading" && <span className="doc-busy"> · reading…</span>}
                  {f.status === "failed" && <span className="doc-busy"> · could not be read</span>}
                </div>
              </div>
              <div className="doc-buttons">
                {viewable(f) && <button type="button" className="doc-btn" onClick={() => { setViewing(f); }}>View</button>}
                <button type="button" className="doc-btn" onClick={() => { download.mutate(f); }}>Download</button>
                {/* Always there; only those given jobs.documents:delete (admins, to start with) may use it. */}
                <button type="button" className="doc-btn danger" aria-label={`Delete ${f.fileName}`} disabled={!canDelete || remove.isPending}
                  title={canDelete ? undefined : "Only admins delete job descriptions: ask one, or for the permission to"}
                  onClick={() => { if (window.confirm(`Delete ${f.fileName}? This cannot be undone.`)) remove.mutate(f); }}>
                  Delete
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {list.length === 0 && !canAdd && files.isSuccess && <p className="muted small">No job description kept yet.</p>}
      {viewing && (
        <FileViewer title={viewing.fileName} path={fileUrl(viewing)} type={viewing.contentType} onClose={() => { setViewing(null); }}
          actions={<button type="button" onClick={() => { download.mutate(viewing); }}>Download</button>} />
      )}
      {canAdd && (
        <DropZone compact accept={JD_TYPES} multiple disabled={upload.isPending}
          label={upload.isPending ? "Uploading…" : "Drop the client's JD to keep it here"}
          hint="PDF, Word, text or a picture" onFiles={(f) => { upload.mutate(f); }} />
      )}
    </div>
  );
}
