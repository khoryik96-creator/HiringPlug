import { useRef } from "react";

import type { Schemas } from "../../api/client";
import { IconButton, PaperclipIcon } from "../ActionIcons";
import { OutlookAttachments } from "./OutlookAttachments";

export const maxDraftFileSize = 10 << 20;

export function OutlookDraftFiles({ draft, disabled, onAdd, onRemove }: {
  draft?: Schemas["OutlookDraft"]; disabled: boolean;
  onAdd: (file: File) => void; onRemove: (attachment: Schemas["OutlookAttachment"]) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const picker = <>
    <IconButton label="Attach file" icon={<PaperclipIcon />} disabled={disabled} onClick={() => { input.current?.click(); }} />
    <input ref={input} type="file" hidden aria-label="Attachment file" disabled={disabled} onChange={(event) => {
      const file = event.currentTarget.files?.[0];
      event.currentTarget.value = "";
      if (file) onAdd(file);
    }} />
  </>;
  // Listed whenever the draft exists: Outlook can say a forward has no attachments
  // while it is still copying them, so the flag alone would hide them.
  return <section className="full outlook-draft-files" aria-label="Draft attachments">
    {draft ? <OutlookAttachments messageId={draft.messageId} tools={picker} quietWhenEmpty remove={{
      disabled, onPick: (file) => { if (window.confirm(`Remove ${file.fileName} from this Outlook draft?`)) onRemove(file); },
    }} /> : <div className="row outlook-draft-file-head">{picker}</div>}
  </section>;
}
