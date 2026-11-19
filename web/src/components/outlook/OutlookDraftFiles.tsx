import { useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { IconButton, TrashIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";
import { fileSize } from "../fileKind";

export const maxDraftFileSize = 10 << 20;

export function OutlookDraftFiles({ draft, disabled, onAdd, onRemove }: {
  draft?: Schemas["OutlookDraft"]; disabled: boolean;
  onAdd: (file: File) => void; onRemove: (attachment: Schemas["OutlookAttachment"]) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const files = useQuery({
    queryKey: ["outlook", "attachments", draft?.messageId],
    enabled: Boolean(draft?.hasAttachments),
    queryFn: async () => unwrap(await api.GET("/v1/outlook/messages/{messageId}/attachments", {
      params: { path: { messageId: draft?.messageId ?? "" } },
    })),
  });
  return <section className="full outlook-attachments outlook-draft-files" aria-label="Draft attachments">
    <button type="button" disabled={disabled} onClick={() => { input.current?.click(); }}>Attach file</button>
    <input ref={input} type="file" hidden aria-label="Attachment file" disabled={disabled} onChange={(event) => {
      const file = event.currentTarget.files?.[0];
      event.currentTarget.value = "";
      if (file) onAdd(file);
    }} />
    {draft?.hasAttachments && <ErrorMessage error={files.error} />}
    {draft?.hasAttachments && files.isPending && <p role="status" className="muted small">Loading attachments…</p>}
    {draft?.hasAttachments && Boolean(files.data?.items.length) && <ul>
      {files.data?.items.map((file) => <li key={file.id}>
        <span>{file.fileName} <small className="muted">{fileSize(file.sizeBytes)}</small></span>
        <IconButton label={`Remove ${file.fileName}`} tip="Remove attachment" icon={<TrashIcon />} danger disabled={disabled}
          onClick={() => { if (window.confirm(`Remove ${file.fileName} from this Outlook draft?`)) onRemove(file); }} />
      </li>)}
    </ul>}
    {draft?.hasAttachments && files.data?.more && <p className="muted small">Open in Outlook to see more attachments.</p>}
  </section>;
}
