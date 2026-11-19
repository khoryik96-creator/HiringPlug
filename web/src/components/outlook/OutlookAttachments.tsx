import { useMutation, useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { downloadFrom } from "../../api/files";
import { ErrorMessage } from "../ErrorMessage";

type Attachment = Schemas["OutlookAttachment"];

function attachmentSize(bytes: number): string {
  if (bytes < 1024) return `${String(bytes)} B`;
  if (bytes < 1024 ** 2) return `${String(Math.round(bytes / 1024))} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

export function OutlookAttachments({ messageId }: { messageId: string }) {
  const attachments = useQuery({
    queryKey: ["outlook", "attachments", messageId],
    queryFn: async ({ signal }) => unwrap(await api.GET("/v1/outlook/messages/{messageId}/attachments", { params: { path: { messageId } }, signal })),
    gcTime: 0,
    retry: false,
  });
  const download = useMutation({
    mutationFn: (file: Attachment) => downloadFrom(
      `/v1/outlook/messages/${encodeURIComponent(messageId)}/attachments/${encodeURIComponent(file.id)}/file`,
      file.fileName,
      "application/octet-stream",
    ),
  });
  return <section className="outlook-attachments" aria-label="Attachments">
    <h4>Attachments</h4>
    {attachments.isPending && <p className="muted" role="status">Loading attachments…</p>}
    <ErrorMessage error={attachments.error}>
      <button type="button" onClick={() => { void attachments.refetch(); }} disabled={attachments.isFetching}>Try again</button>
    </ErrorMessage>
    <ErrorMessage error={download.error} />
    {attachments.data?.items.length === 0 && !attachments.data.more && <p className="muted">No files attached.</p>}
    {!!attachments.data?.items.length && <ul>
      {attachments.data.items.map((file) => <li key={file.id}>
        <span><strong>{file.fileName}</strong> <small className="muted">{attachmentSize(file.sizeBytes)}</small></span>
        {file.downloadable ? <button
          type="button"
          aria-label={`Download ${file.fileName}`}
          disabled={download.isPending}
          onClick={() => { download.mutate(file); }}
        >{download.isPending && download.variables.id === file.id ? "Downloading…" : "Download"}</button>
          : <small className="muted">Available in Outlook</small>}
      </li>)}
    </ul>}
    {attachments.data?.more && <p className="muted">Open in Outlook to see the remaining attachments.</p>}
  </section>;
}
