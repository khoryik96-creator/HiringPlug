// "Save as a new template" under an email being written: its subject and
// message become a template for everyone, the candidate's details put back
// as fields. Changing the company's templates stays with admins (Settings).
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { api, unwrap } from "../../api/client";
import { emailTemplatesKey } from "../../api/keys";
import { filledIn } from "../../requiredFields";
import { ErrorMessage } from "../ErrorMessage";
import { valuesToTokens, withoutCaretMarks } from "./richText";

export function SaveAsTemplate({ subject, html, values }: { subject: string; html: string; values: Record<string, string> | undefined }) {
  const queryClient = useQueryClient();
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  const nameField = useRef<HTMLInputElement>(null);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/email-templates", {
      // The message's filled-in values are turned back into fields by the server.
      body: { name: name.trim(), subject: valuesToTokens(subject, values ?? {}), bodyHtml: withoutCaretMarks(html) },
    })),
    onSuccess: async () => { setNaming(false); await queryClient.invalidateQueries({ queryKey: emailTemplatesKey }); },
  });
  if (!naming) {
    return (
      <span className="save-template">
        {save.isSuccess && <span className="muted small" role="status">Saved as &ldquo;{save.data.name}&rdquo;. </span>}
        <button type="button" className="link" onClick={() => { setName(""); save.reset(); setNaming(true); }}>Save as a new template</button>
      </span>
    );
  }
  return (
    <span className="save-template row">
      <input ref={nameField} required aria-label="New template name" placeholder="Template name" value={name} autoFocus onChange={(e) => { setName(e.target.value); }} />
      <button type="button" disabled={save.isPending} onClick={() => { if (filledIn(nameField.current)) save.mutate(); }}>Save template</button>
      <button type="button" className="link" onClick={() => { setNaming(false); }}>Cancel</button>
      <ErrorMessage error={save.error} />
    </span>
  );
}
