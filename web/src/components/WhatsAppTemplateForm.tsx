// A WhatsApp template's name and text, with field chips that put a field such
// as {{candidate.firstName}} where the cursor is. Used in Settings › WhatsApp
// templates and in the WhatsApp window.
import { useRef, useState } from "react";

import { ErrorMessage } from "./ErrorMessage";
import type { TemplateValues } from "./useWhatsAppTemplates";
import { useInsertAtCursor } from "./whatsAppText";

export function WhatsAppTemplateForm({ initial, fields, saving, error, onSave, onCancel, onDelete, bodyLabel = "Message", saveLabel = "Save" }: {
  initial: TemplateValues; fields: { key: string; label: string }[]; saving: boolean; error: unknown;
  onSave: (v: TemplateValues) => void; onCancel?: () => void; onDelete?: () => void;
  bodyLabel?: string; saveLabel?: string;
}) {
  const [name, setName] = useState(initial.name);
  const [body, setBody] = useState(initial.body);
  const box = useRef<HTMLTextAreaElement>(null);
  const insert = useInsertAtCursor(box, body, setBody);
  return (
    <form className="form-grid" onSubmit={(e) => { e.preventDefault(); onSave({ name: name.trim(), body }); }}>
      <label className="field">Name<input required maxLength={100} value={name} onChange={(e) => { setName(e.target.value); }} /></label>
      <label className="field full">{bodyLabel}
        <textarea ref={box} required rows={5} maxLength={4000} value={body} onChange={(e) => { setBody(e.target.value); }} />
      </label>
      {fields.length > 0 && (
        <div className="full whatsapp-fields" role="group" aria-label="Fields">
          <span className="muted small">Insert at the cursor:</span>
          {fields.map((f) => (
            <button key={f.key} type="button" className="chip" title={`Puts {{${f.key}}} where the cursor is`} onClick={() => { insert(`{{${f.key}}}`); }}>{f.label}</button>
          ))}
        </div>
      )}
      <ErrorMessage error={error} />
      <div className="row full">
        <button type="submit" disabled={saving}>{saveLabel}</button>
        {onCancel && <button type="button" className="link" onClick={onCancel}>Cancel</button>}
        {onDelete && <button type="button" className="link danger" disabled={saving} onClick={onDelete}>Delete</button>}
      </div>
    </form>
  );
}
