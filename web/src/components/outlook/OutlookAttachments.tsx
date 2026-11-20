import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { downloadFrom, outlookAttachmentFilePath } from "../../api/files";
import { IconButton, TrashIcon } from "../ActionIcons";
import { isCvFileName } from "../DropZone";
import { ErrorMessage } from "../ErrorMessage";
import { OutlookSaveCv } from "./OutlookSaveCv";

type Attachment = Schemas["OutlookAttachment"];

function attachmentSize(bytes: number): string {
  if (bytes < 1024) return `${String(bytes)} B`;
  if (bytes < 1024 ** 2) return `${String(Math.round(bytes / 1024))} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

export function OutlookAttachments({ messageId, canSaveCv = false, canImportCv = false, candidateId, remove }: {
  messageId: string;
  canSaveCv?: boolean;
  canImportCv?: boolean;
  candidateId?: string;
  remove?: { disabled: boolean; onPick: (file: Attachment) => void };
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
      outlookAttachmentFilePath(messageId, file.id),
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
        <div className="row outlook-attachment-buttons">
          {file.downloadable ? <>
            <button type="button" aria-label={`Download ${file.fileName}`} disabled={download.isPending}
              onClick={() => { download.mutate(file); }}>
              {download.isPending && download.variables.id === file.id ? "Downloading…" : "Download"}
            </button>
            {canSaveCv && file.savable && isCvFileName(file.fileName) &&
              <button type="button" aria-label={`Save ${file.fileName} to candidate`} disabled={chosen !== undefined}
                onClick={() => { setSaveId(file.id); }}>Save CV</button>}
            {canImportCv && file.savable && isCvFileName(file.fileName) && <a className="button"
              aria-label={`Import ${file.fileName} as a candidate`}
              href={`#/import?${new URLSearchParams({ outlookMessage: messageId, outlookAttachment: file.id }).toString()}`}>Import CV</a>}
          </> : <small className="muted">Available in Outlook</small>}
          {remove && <IconButton label={`Remove ${file.fileName}`} tip="Remove attachment" icon={<TrashIcon />} danger
            disabled={remove.disabled} onClick={() => { remove.onPick(file); }} />}
        </div>
        {canSaveCv && chosen?.id === file.id && <OutlookSaveCv fileName={file.fileName}
          filePath={outlookAttachmentFilePath(messageId, file.id)} candidateId={candidateId}
          onClose={() => { setSaveId(undefined); }} />}
      </li>)}
    </ul>}
    {attachments.data?.more && <p className="muted">Open in Outlook to see the remaining attachments.</p>}
  </section>;
}
