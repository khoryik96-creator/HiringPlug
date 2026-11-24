// "Save as template" in the WhatsApp window: the message being written becomes
// a template for anyone, with this person's details turned back into fields
// ({{candidate.firstName}}...). The text and a name are checked before saving.
import type { Schemas } from "../api/client";
import { useWhatsAppTemplateChanges } from "./useWhatsAppTemplates";
import { WhatsAppTemplateForm } from "./WhatsAppTemplateForm";
import { valuesToFields } from "./whatsAppText";

export function WhatsAppSaveTemplate({ text, fields, onSaved, onCancel }: {
  text: string; fields: Schemas["WhatsAppFieldValue"][]; onSaved: (name: string) => void; onCancel: () => void;
}) {
  const { create } = useWhatsAppTemplateChanges();
  return (
    <section className="whatsapp-pane" aria-label="Save as template">
      <h4>Save as template</h4>
      <p className="muted small">Their details are turned back into fields such as {"{{candidate.firstName}}"}, so the template works for anyone. Check the text and give it a name.</p>
      <WhatsAppTemplateForm initial={{ name: "", body: valuesToFields(text, fields) }} fields={fields} bodyLabel="Template text" saveLabel="Save template"
        saving={create.isPending} error={create.error}
        onSave={(v) => { create.mutate(v, { onSuccess: (t) => { onSaved(t.name); } }); }} onCancel={onCancel} />
    </section>
  );
}
