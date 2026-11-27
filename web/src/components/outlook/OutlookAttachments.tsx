import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { downloadFrom, outlookAttachmentFilePath, outlookAttachmentsZipPath } from "../../api/files";
import { DownloadIcon, IconButton, TrashIcon } from "../ActionIcons";
import { isCvFileName } from "../DropZone";
import { ErrorMessage } from "../ErrorMessage";
import { FileViewer } from "../FileViewer";
import { useCan } from "../useCan";
import { CreateJobButton, isJdFileName } from "./CreateJobButton";
import { ImportCvButton } from "./ImportCvButton";

type Attachment = Schemas["OutlookAttachment"];

/** What the viewer can show in the app (as Outlook previews them): a PDF or a picture; null for anything else. */
export function viewableType(fileName: string): string | null {
  const ext = fileName.toLowerCase().split(".").pop() ?? "";
  if (ext === "pdf") return "application/pdf";
  const pictures: Record<string, string> = { png: "image/png", jpg: "image/jpeg", jpeg: "image/jpeg", gif: "image/gif", webp: "image/webp" };
  return pictures[ext] ?? null;
}

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
  // As in Outlook: with two files or more, all of them at once in a .zip (not in a draft being written).
  const all = useMutation({
    mutationFn: () => downloadFrom(outlookAttachmentsZipPath(messageId), "Attachments.zip", "application/zip"),
  });
  const [viewing, setViewing] = useState<Attachment | null>(null);
  const several = !remove && (attachments.data?.items.filter((f) => f.downloadable).length ?? 0) > 1;
  const downloadAll = several && <button type="button" className="secondary small" disabled={all.isPending}
    onClick={() => { all.mutate(); }}><DownloadIcon /> {all.isPending ? "Downloading…" : "Download all"}</button>;
  return <section className="outlook-attachments" aria-label="Attachments">
    {tools ? <div className="row outlook-draft-file-head"><h4>Attachments</h4>{tools}</div>
      : <div className="row outlook-draft-file-head"><h4>Attachments</h4>{downloadAll}</div>}
    <ErrorMessage error={all.error} />
    {attachments.isPending && <p className="muted" role="status">Loading attachments…</p>}
    <ErrorMessage error={attachments.error}>
      <button type="button" onClick={() => { void attachments.refetch(); }} disabled={attachments.isFetching}>Try again</button>
    </ErrorMessage>
    <ErrorMessage error={download.error} />
    {!quietWhenEmpty && attachments.data?.items.length === 0 && !attachments.data.more && <p className="muted">No files attached.</p>}
    {!!attachments.data?.items.length && <ul>
      {attachments.data.items.map((file) => <li key={file.id}>
        <span>{file.downloadable && viewableType(file.fileName)
          ? <button type="button" className="link outlook-attachment-open" title="Open to view" onClick={() => { setViewing(file); }}><strong>{file.fileName}</strong></button>
          : <strong>{file.fileName}</strong>} <small className="muted">{attachmentSize(file.sizeBytes)}</small></span>
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
    {viewing && <FileViewer title={viewing.fileName} path={outlookAttachmentFilePath(messageId, viewing.id)}
      type={viewableType(viewing.fileName) ?? "application/pdf"} onClose={() => { setViewing(null); }}
      actions={<button type="button" disabled={download.isPending} onClick={() => { download.mutate(viewing); }}><DownloadIcon /> Download</button>} />}
  </section>;
}
