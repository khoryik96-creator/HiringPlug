import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type InputHTMLAttributes, useState } from "react";

import { api, type Contact, unwrap } from "../api/client";
import { contactKey, contactsKey } from "../api/keys";
import { CONTACT_STATUS, keysOf } from "../labels";
import { CompanyPicker, type PickedCompany } from "./CompanyPicker";
import { CONTACT_PATHS, contactFields, type ContactFormState, contactProblems, formFromContact } from "./contactFormState";
import { Field } from "./Field";
import { fieldErrors, FormInvalid, sameBody } from "./forms";
import { SaveError } from "./SaveError";

type TextKey = "firstName" | "lastName" | "title" | "email" | "phone" | "mobile" | "linkedinUrl";

/**
 * Add a contact (contact = null, optionally at `company`) or edit one. Edits
 * save with the version they were opened with.
 */
export function ContactForm({ contact, company = null, onDone, onSaved }: {
  contact: Contact | null;
  company?: PickedCompany | null;
  onDone: () => void;
  onSaved?: (contact: Contact) => void;
}) {
  const queryClient = useQueryClient();
  const [opened] = useState(() => ({ id: contact?.id, version: contact?.version ?? 0, form: formFromContact(contact, company) }));
  const [form, setForm] = useState(opened.form);
  const save = useMutation({
    mutationFn: async () => {
      const problems = contactProblems(form);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const fields = contactFields(form);
      if (!opened.id) return unwrap(await api.POST("/v1/contacts", { body: fields }));
      if (sameBody(fields, contactFields(opened.form))) return null; // nothing changed
      return unwrap(await api.PATCH("/v1/contacts/{contactId}", {
        params: { path: { contactId: opened.id } }, body: { ...fields, version: opened.version },
      }));
    },
    onSuccess: (saved) => {
      if (saved) {
        queryClient.setQueryData(contactKey(saved.id), saved);
        void queryClient.invalidateQueries({ queryKey: contactsKey });
        onSaved?.(saved);
      }
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof ContactFormState>(key: K, value: ContactFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };
  const text = (key: TextKey, label: string, extra: InputHTMLAttributes<HTMLInputElement> = {}) => (
    <Field label={label} error={errors[CONTACT_PATHS[key]]}>
      <input value={form[key]} onChange={(e) => { set(key, e.target.value); }} {...extra} />
    </Field>
  );

  return (
    <form
      className="panel form-grid"
      aria-label={opened.id ? "Edit contact" : "Add contact"}
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <Field label="Company" error={errors[CONTACT_PATHS.company]}>
        <CompanyPicker value={form.company} onChange={(c) => { set("company", c); }} />
      </Field>
      {text("firstName", "First name")}
      {text("lastName", "Last name")}
      {text("title", "Job title", { placeholder: "e.g. HR Manager" })}
      {/* Not type="email": the browser's rule refuses addresses the API accepts. */}
      {text("email", "Email", { inputMode: "email", autoComplete: "off", spellCheck: false })}
      {text("phone", "Office phone")}
      {text("mobile", "Mobile")}
      {text("linkedinUrl", "LinkedIn", { placeholder: "linkedin.com/in/name" })}
      <Field label="Status" error={errors[CONTACT_PATHS.status]}>
        <select value={form.status} onChange={(e) => { set("status", e.target.value as ContactFormState["status"]); }}>
          {keysOf(CONTACT_STATUS).map((s) => <option key={s} value={s}>{CONTACT_STATUS[s]}</option>)}
        </select>
      </Field>
      <div className="full">
        <Field label="Notes" error={errors[CONTACT_PATHS.notes]}>
          <textarea rows={3} value={form.notes} onChange={(e) => { set("notes", e.target.value); }} />
        </Field>
      </div>

      <div className="row full">
        <button type="submit" disabled={save.isPending}>{opened.id ? "Save" : "Add contact"}</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError
        error={save.error}
        shown={Object.values(CONTACT_PATHS)}
        onReload={() => {
          if (opened.id) void queryClient.invalidateQueries({ queryKey: contactKey(opened.id) });
          onDone();
        }}
      />
    </form>
  );
}
