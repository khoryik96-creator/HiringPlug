import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { downloadFrom } from "../../api/files";
import { CV_TYPES } from "../DropZone";
import { ErrorMessage } from "../ErrorMessage";
import { OutlookSaveCv } from "./OutlookSaveCv";

type Attachment = Schemas["OutlookAttachment"];
const cvExtensions = CV_TYPES.split(",");

function attachmentPath(messageId: string, attachmentId: string) {
  return `/v1/outlook/messages/${encodeURIComponent(messageId)}/attachments/${encodeURIComponent(attachmentId)}/file`;
}

function attachmentSize(bytes: number): string {
  if (bytes < 1024) return `${String(bytes)} B`;
  if (bytes < 1024 ** 2) return `${String(Math.round(bytes / 1024))} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

export function OutlookAttachments({ messageId, canSaveCv = false, candidateId }: {
  messageId: string;
  canSaveCv?: boolean;
  candidateId?: string;
}) {
  const [saveId, setSaveId] = useState<string>();
  const attachments = useQuery({
    queryKey: ["outlook", "attachments", messageId],
    queryFn: async ({ signal }) => unwrap(await api.GET("/v1/outlook/messages/{messageId}/attachments", { params: { path: { messageId } }, signal })),
    gcTime: 0,
    retry: false,
  });
  const download = useMutation({
    mutationFn: (file: Attachment) => downloadFrom(
      attachmentPath(messageId, file.id),
      file.fileName,
      "application/octet-stream",
    ),
  });
  const chosen = attachments.data?.items.find((file) => file.id === saveId);
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
        {file.downloadable ? <div className="row outlook-attachment-buttons">
          <button type="button" aria-label={`Download ${file.fileName}`} disabled={download.isPending}
            onClick={() => { download.mutate(file); }}>
            {download.isPending && download.variables.id === file.id ? "Downloading…" : "Download"}
          </button>
          {canSaveCv && cvExtensions.some((extension) => file.fileName.toLowerCase().endsWith(extension)) &&
            <button type="button" aria-label={`Save ${file.fileName} to candidate`} disabled={chosen !== undefined}
              onClick={() => { setSaveId(file.id); }}>Save CV</button>}
        </div> : <small className="muted">Available in Outlook</small>}
        {canSaveCv && chosen?.id === file.id && <OutlookSaveCv fileName={file.fileName}
          filePath={attachmentPath(messageId, file.id)} candidateId={candidateId}
          onClose={() => { setSaveId(undefined); }} />}
      </li>)}
    </ul>}
    {attachments.data?.more && <p className="muted">Open in Outlook to see the remaining attachments.</p>}
  </section>;
}
