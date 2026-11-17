import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type InputHTMLAttributes, useEffect, useState } from "react";

import { api, type Contact, unwrap } from "../api/client";
import { contactKey, contactsKey } from "../api/keys";
import { CONTACT_STATUS, keysOf } from "../labels";
import { CompanyPicker, type PickedCompany } from "./CompanyPicker";
import { ContactFromLinkedin } from "./ContactFromLinkedin";
import { afterContactAdded } from "./contactLinkedin";
import { CONTACT_PATHS, contactFields, type ContactFormState, contactProblems, formFromContact } from "./contactFormState";
import { Field } from "./Field";
import { useFieldSettings } from "./useFieldSettings";
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
  const fs = useFieldSettings();
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
      // A new contact: an email guessed (then verified) and their time at the
      // company checked on LinkedIn, while you go on working.
      if (saved && !opened.id) {
        void afterContactAdded(saved).catch(() => undefined).finally(() => {
          void queryClient.invalidateQueries({ queryKey: contactsKey });
          void queryClient.invalidateQueries({ queryKey: contactKey(saved.id) });
        });
      }
      if (saved) {
        queryClient.setQueryData(contactKey(saved.id), saved);
        void queryClient.invalidateQueries({ queryKey: contactsKey });
        onSaved?.(saved);
      }
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const taken = useEmailTaken(form.email, opened.id);
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
      {!opened.id && <>
        {text("linkedinUrl", "LinkedIn profile", { placeholder: "linkedin.com/in/name" })}
        <ContactFromLinkedin url={form.linkedinUrl} company={form.company}
          onRead={(fill) => {
            setForm((f) => ({ ...f, firstName: f.firstName || fill.firstName, lastName: f.lastName || fill.lastName,
              title: f.title || fill.title, linkedinUrl: fill.linkedinUrl }));
          }}
          onCompany={(c) => { set("company", c); }} />
      </>}
      {text("firstName", "First name")}
      {text("lastName", "Last name")}
      {text("title", "Job title", { placeholder: "e.g. HR Manager" })}
      {/* Not type="email": the browser's rule refuses addresses the API accepts. */}
      {text("email", "Email", { inputMode: "email", autoComplete: "off", spellCheck: false })}
      {taken && (
        <p className="warning small full" role="alert">
          Already a contact: <a href={`#/contacts/${taken.id}`}>{taken.fullName}</a>{taken.companyName ? ` at ${taken.companyName}` : ""}.
        </p>
      )}
      {text("phone", "Office phone")}
      {text("mobile", "Mobile")}
      {opened.id && text("linkedinUrl", "LinkedIn", { placeholder: "linkedin.com/in/name" })}
      <Field label="Role" error={errors[CONTACT_PATHS.role]}>
        <select value={form.role} onChange={(e) => { set("role", e.target.value); }}>
          <option value="">Not set</option>
          {fs.options("contact_role", form.role || null).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
        </select>
      </Field>
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

/** Another contact with this email, as soon as it is typed (one email, one contact). */
function useEmailTaken(email: string, self: string | undefined) {
  const typed = email.trim().toLowerCase();
  const [settled, setSettled] = useState("");
  useEffect(() => {
    const t = setTimeout(() => { setSettled(/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(typed) ? typed : ""); }, 400);
    return () => { clearTimeout(t); };
  }, [typed]);
  const found = useQuery({
    queryKey: [...contactsKey, "email", settled],
    queryFn: async () => unwrap(await api.GET("/v1/contacts", { params: { query: { q: settled, limit: 5 } } })).items,
    enabled: settled !== "",
    staleTime: 30_000,
  });
  return settled === typed ? found.data?.find((c) => c.email?.toLowerCase() === settled && c.id !== self) : undefined;
}
