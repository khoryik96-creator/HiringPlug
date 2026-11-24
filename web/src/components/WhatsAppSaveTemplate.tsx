// "Save as template" in the WhatsApp window: the message being written becomes
// a template for anyone, with this person's details turned back into fields
// ({{candidate.firstName}}...). The text and a name are checked before saving,
// and any of their details still in the text is pointed out.
import type { Schemas } from "../api/client";
import { useWhatsAppTemplateChanges } from "./useWhatsAppTemplates";
import { WhatsAppTemplateForm } from "./WhatsAppTemplateForm";
import { valuesLeft, valuesToFields, valueToField } from "./whatsAppText";

/**
 * `sourceKeys`: the fields the template the message came from used, turned
 * back too. `onSaved` runs even if the pane was left while saving.
 */
export function WhatsAppSaveTemplate({ text, fields, sourceKeys, onSaved, onCancel }: {
  text: string; fields: Schemas["WhatsAppFieldValue"][]; sourceKeys: string[]; onSaved: (name: string) => void; onCancel: () => void;
}) {
  const { create } = useWhatsAppTemplateChanges({ onCreated: (t) => { onSaved(t.name); } });
  return (
    <section className="whatsapp-pane" aria-label="Save as template">
      <h4>Save as template</h4>
      <p className="muted small">Their details are turned back into fields such as {"{{candidate.firstName}}"}, so the template works for anyone. Check the text and give it a name.</p>
      <WhatsAppTemplateForm initial={{ name: "", body: valuesToFields(text, fields, sourceKeys) }} fields={fields} bodyLabel="Template text" saveLabel="Save template"
        saving={create.isPending} error={create.error} onSave={(v) => { create.mutate(v); }} onCancel={onCancel}
        notes={(body, setBody) => valuesLeft(body, fields).map((f) => (
          <p key={f.key} className="full warning small" role="status">
            Still has {f.label}: {f.value} — <button type="button" className="link" onClick={() => { setBody(valueToField(body, f.value, f.key)); }}>replace with the field?</button>
          </p>
        ))} />
    </section>
  );
}
