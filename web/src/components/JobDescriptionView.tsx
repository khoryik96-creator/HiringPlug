// A job's description on Job details (and the job's ads): the latest uploaded
// job description file, shown as it looks, like a CV (a PDF or picture as it
// is; a Word, RTF or ODT file as its PDF copy), else the description pasted in.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { jobJdFilesKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { useFileUrl } from "./FileViewer";

type JdFile = Schemas["JdFile"];
const inline = (f: JdFile) => f.contentType === "application/pdf" || f.contentType.startsWith("image/");

/** The job's job description files, newest first (refreshed while one is read or its PDF made). */
export function useJdFiles(jobId: string) {
  return useQuery({
    queryKey: jobJdFilesKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/jd-files", { params: { path: { jobId } } })).items,
    // Every 2 s while a file is read or its PDF made, for up to 3 minutes (a worker that is down does not poll forever).
    refetchInterval: (q) => (q.state.dataUpdateCount < 90 && q.state.data?.some((f) => f.status === "reading" || f.pdfStatus === "queued") ? 2000 : false),
  });
}

export function JobDescriptionView({ jobId, pasted }: { jobId: string; pasted: string | null }) {
  const files = useJdFiles(jobId);
  const latest = files.data?.[0];
  if (!files.data && !files.error) return <p className="muted">Loading…</p>;
  if (!latest) return <p className="pre job-description">{pasted ?? <span className="muted">No description yet.</span>}</p>;
  return <JdFilePreview jobId={jobId} file={latest} />;
}

/** One uploaded job description as it looks. */
export function JdFilePreview({ jobId, file }: { jobId: string; file: JdFile }) {
  const queryClient = useQueryClient();
  // A Word file uploaded before previews began: its PDF copy is made once, on first view.
  const make = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/jd-files/{jdFileId}/pdf", { params: { path: { jdFileId: file.id } } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: jobJdFilesKey(jobId) }),
  });
  const office = !inline(file) && file.contentType !== "text/plain";
  const needsCopy = office && file.pdfStatus === null;
  const { mutate, isIdle } = make;
  useEffect(() => { if (needsCopy && isIdle) mutate(); }, [needsCopy, isIdle, mutate]);
  return (
    <div className="jd-preview">
      <p className="muted small">From the uploaded file <strong>{file.fileName}</strong></p>
      {inline(file)
        ? <InlineFile path={`/v1/jd-files/${file.id}/file`} type={file.contentType} title={file.fileName} />
        : file.pdfStatus === "ready"
          ? <InlineFile path={`/v1/jd-files/${file.id}/pdf`} type="application/pdf" title={file.fileName} />
          : <>
            {/* Its text meanwhile: a preview being made (or one that never comes) does not hide it. */}
            {office && (file.pdfStatus === "queued" || (needsCopy && !make.isError)) && <p className="muted small" role="status">Making a preview of the Word file… its text meanwhile:</p>}
            <ErrorMessage error={make.error} />
            {file.details?.description
              ? <p className="pre job-description cv-like">{file.details.description}</p>
              : <p className="muted">{file.status === "reading" ? "Reading the file…" : "The file's text could not be shown: download it below to see it."}</p>}
          </>}
    </div>
  );
}

/** A PDF or picture inside the page, as the CV preview shows one. */
function InlineFile({ path, type, title }: { path: string; type: string; title: string }) {
  const { url, error } = useFileUrl(path, type);
  if (error) return <ErrorMessage error={error} />;
  if (!url) return <p className="muted">Making the preview…</p>;
  return type.startsWith("image/")
    ? <img className="jd-preview-body" src={url} alt={title} />
    : <iframe className="jd-preview-body" title={title} src={`${url}#navpanes=0&view=FitH`} />;
}
