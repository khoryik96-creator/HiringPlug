// Where a submitted claim's receipt went in the company's claims storage:
// a link to it once uploaded, or that it is on its way or failed (and why).
import type { Schemas } from "../../api/client";

export function UploadNote({ upload }: { upload: Schemas["ClaimUpload"] }) {
  switch (upload.status) {
    case "done":
      return (
        <span className="sub claim-upload done" title={upload.path ?? undefined}>
          {upload.url ? <a href={upload.url} target="_blank" rel="noreferrer noopener">In the drive</a> : "In the drive"}
        </span>
      );
    case "pending":
      return <span className="sub claim-upload muted">Uploading to the drive…</span>;
    case "failed":
      return <span className="sub claim-upload error" title={upload.error ?? undefined}>Not uploaded: {upload.error ?? "the upload failed"}</span>;
    case "none":
      return null;
  }
}
