import { useRef } from "react";

import type { Schemas } from "../../api/client";
import { OutlookAttachments } from "./OutlookAttachments";

export const maxDraftFileSize = 10 << 20;

export function OutlookDraftFiles({ draft, disabled, onAdd, onRemove }: {
  draft?: Schemas["OutlookDraft"]; disabled: boolean;
  onAdd: (file: File) => void; onRemove: (attachment: Schemas["OutlookAttachment"]) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  return <section className="full outlook-draft-files" aria-label="Draft attachments">
    <button type="button" disabled={disabled} onClick={() => { input.current?.click(); }}>Attach file</button>
    <input ref={input} type="file" hidden aria-label="Attachment file" disabled={disabled} onChange={(event) => {
      const file = event.currentTarget.files?.[0];
      event.currentTarget.value = "";
      if (file) onAdd(file);
    }} />
    {draft?.hasAttachments && <OutlookAttachments messageId={draft.messageId} remove={{
      disabled,
      onPick: (file) => { if (window.confirm(`Remove ${file.fileName} from this Outlook draft?`)) onRemove(file); },
    }} />}
  </section>;
}
