import { useMutation, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { downloadFrom, outlookAttachmentFilePath } from "../../api/files";
import { DownloadIcon, IconButton, TrashIcon } from "../ActionIcons";
import { isCvFileName } from "../DropZone";
import { ErrorMessage } from "../ErrorMessage";
import { useCan } from "../useCan";
import { CreateJobButton, isJdFileName } from "./CreateJobButton";
import { ImportCvButton } from "./ImportCvButton";

type Attachment = Schemas["OutlookAttachment"];

function attachmentSize(bytes: number): string {
  if (bytes < 1024) return `${String(bytes)} B`;
  if (bytes < 1024 ** 2) return `${String(Math.round(bytes / 1024))} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

export function OutlookAttachments({ messageId, canImportCv = false, userId, remove, tools, quietWhenEmpty = false }: {
  messageId: string;
  /** A draft: no "No files attached." line when it has none. */
  quietWhenEmpty?: boolean;
  canImportCv?: boolean;
  /** Who imports a CV (Import CV shows only with one). */
  userId?: string;
  remove?: { disabled: boolean; onPick: (file: Attachment) => void };
  tools?: ReactNode;
}) {
  const canCreateJob = useCan("jobs:create");
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
  return <section className="outlook-attachments" aria-label="Attachments">
    {tools ? <div className="row outlook-draft-file-head"><h4>Attachments</h4>{tools}</div> : <h4>Attachments</h4>}
    {attachments.isPending && <p className="muted" role="status">Loading attachments…</p>}
    <ErrorMessage error={attachments.error}>
      <button type="button" onClick={() => { void attachments.refetch(); }} disabled={attachments.isFetching}>Try again</button>
    </ErrorMessage>
    <ErrorMessage error={download.error} />
    {!quietWhenEmpty && attachments.data?.items.length === 0 && !attachments.data.more && <p className="muted">No files attached.</p>}
    {!!attachments.data?.items.length && <ul>
      {attachments.data.items.map((file) => <li key={file.id}>
        <span><strong>{file.fileName}</strong> <small className="muted">{attachmentSize(file.sizeBytes)}</small></span>
        <div className="row outlook-attachment-buttons">
          {file.downloadable ? <>
            <IconButton label={`Download ${file.fileName}`} tip={download.isPending && download.variables.id === file.id ? "Downloading…" : "Download"}
              icon={<DownloadIcon />} disabled={download.isPending} onClick={() => { download.mutate(file); }} />
            {canImportCv && userId && file.savable && isCvFileName(file.fileName) && <ImportCvButton userId={userId} messageId={messageId} file={file} />}
            {!remove && canCreateJob && isJdFileName(file.fileName) && <CreateJobButton messageId={messageId} file={file} />}
          </> : <small className="muted">Available in Outlook</small>}
          {remove && <IconButton label={`Remove ${file.fileName}`} tip="Remove attachment" icon={<TrashIcon />} danger
            disabled={remove.disabled} onClick={() => { remove.onPick(file); }} />}
        </div>
      </li>)}
    </ul>}
    {attachments.data?.more && <p className="muted">Open in Outlook to see the remaining attachments.</p>}
  </section>;
}
