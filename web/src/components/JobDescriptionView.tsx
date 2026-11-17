// A job's description on Job details: the latest uploaded job description
// file, shown like a CV (a PDF or picture as it is; a Word file as the text
// read from it), else the description pasted in.
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { fetchFile } from "../api/files";
import { jobJdFilesKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

type JdFile = Schemas["JdFile"];
const inline = (f: JdFile) => f.contentType === "application/pdf" || f.contentType.startsWith("image/");

export function JobDescriptionView({ jobId, pasted }: { jobId: string; pasted: string | null }) {
  const files = useQuery({
    queryKey: jobJdFilesKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/jd-files", { params: { path: { jobId } } })).items,
    refetchInterval: (q) => (q.state.data?.some((f) => f.status === "reading") ? 2000 : false),
  });
  const latest = files.data?.[0];
  if (!files.data && !files.error) return <p className="muted">Loading…</p>;
  if (!latest) return <p className="pre job-description">{pasted ?? <span className="muted">No description yet.</span>}</p>;
  return (
    <div className="jd-preview">
      <p className="muted small">From the uploaded file <strong>{latest.fileName}</strong></p>
      {inline(latest)
        ? <InlineFile path={`/v1/jd-files/${latest.id}/file`} type={latest.contentType} title={latest.fileName} />
        : latest.details?.description
          ? <p className="pre job-description cv-like">{latest.details.description}</p>
          : <p className="muted">{latest.status === "reading" ? "Reading the file…" : "The file's text could not be shown: download it below to see it."}</p>}
    </div>
  );
}

/** A PDF or picture inside the page, as the CV preview shows one. */
function InlineFile({ path, type, title }: { path: string; type: string; title: string }) {
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let made: string | null = null;
    let live = true;
    fetchFile(path, type)
      .then((blob) => { if (live) { made = URL.createObjectURL(blob); setUrl(made); } })
      .catch((e: unknown) => { if (live) setError(e); });
    return () => { live = false; if (made) URL.revokeObjectURL(made); };
  }, [path, type]);
  if (error) return <ErrorMessage error={error} />;
  if (!url) return <p className="muted">Making the preview…</p>;
  return type.startsWith("image/")
    ? <img className="jd-preview-body" src={url} alt={title} />
    : <iframe className="jd-preview-body" title={title} src={`${url}#navpanes=0&view=FitH`} />;
}
