import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import { Field } from "../Field";
import { fieldErrors } from "../forms";
import { outlookAddresses } from "./outlookAddresses";

export function OutlookForward({ messageId, onOpen, onClose }: {
  messageId: string; onOpen: (draftId: string) => void; onClose: () => void;
}) {
  const client = useQueryClient();
  const [to, setTo] = useState("");
  const [note, setNote] = useState("");
  const recipients = outlookAddresses(to);
  const forward = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/outlook/messages/{messageId}/forward", {
      params: { path: { messageId } }, body: { to: recipients, note: note.trim() || undefined },
    })),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ["outlook", "list"] }); },
  });
  const errors = fieldErrors(forward.error);
  const recipientError = errors.to ?? Object.entries(errors).find(([field]) => field.startsWith("to.") || field.startsWith("to["))?.[1];
  return <form className="outlook-reply" onSubmit={(event) => {
    event.preventDefault();
    if (!recipients.length || forward.isPending) return;
    forward.mutate(undefined, { onSuccess: (draft) => { onClose(); onOpen(draft.messageId); } });
  }}>
    <fieldset className="outlook-draft-fields" disabled={forward.isPending}>
      <Field label="To" error={recipientError}>
        <input autoFocus required value={to} onChange={(event) => { setTo(event.target.value); }} />
      </Field>
      <Field label="Note (optional)" error={errors.note}>
        <textarea rows={3} maxLength={10000} value={note} onChange={(event) => { setNote(event.target.value); }} />
      </Field>
      <ErrorMessage error={forward.error} />
      <div className="row outlook-actions">
        <button type="submit" className="primary" disabled={!recipients.length}>{forward.isPending ? "Creating draft…" : "Review forward"}</button>
        <button type="button" onClick={onClose}>Cancel</button>
      </div>
    </fieldset>
  </form>;
}
