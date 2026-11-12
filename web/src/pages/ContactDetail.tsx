import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { contactKey } from "../api/keys";
import { ContactBanner } from "../components/ContactBanner";
import { ContactEmails } from "../components/ContactEmails";
import { ContactForm } from "../components/ContactForm";
import { EmailCheckBadge } from "../components/EmailCheckBadge";
import { EmploymentCheck } from "../components/EmploymentCheck";
import { ErrorMessage } from "../components/ErrorMessage";
import { type Section, SectionTabs } from "../components/SectionTabs";
import { roleLabel } from "../components/contactRole";
import { Timeline } from "../components/Timeline";
import { useFieldSettings } from "../components/useFieldSettings";
import { CONTACT_STATUS } from "../labels";

type Tab = "details" | "employment" | "email" | "activity";

/**
 * One contact, laid out like a candidate: their card (email and phones on it)
 * and tabs for the rest, each fact shown once. Render it with key={id}.
 */
export function ContactDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const [editing, setEditing] = useState(false);
  const [active, setActive] = useState<Tab>("details");
  const [writing, setWriting] = useState(false);
  const contact = useQuery({
    queryKey: contactKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/contacts/{contactId}", { params: { path: { contactId: id } } })),
    // Follow an employment check still being read.
    refetchInterval: (q) => (q.state.data?.employment?.status === "reading" ? 2000 : false),
  });
  const recheck = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/contacts/{contactId}/email-check", { params: { path: { contactId: id } } })),
    onSuccess: (updated) => { queryClient.setQueryData(contactKey(id), updated); },
  });
  const c = contact.data;
  if (!c) return contact.error ? <ErrorMessage error={contact.error} /> : <p className="muted">Loading…</p>;
  const canEdit = permissions.includes("contacts:update");
  const canEmail = permissions.includes("emails:read") && permissions.includes("emails:send");

  const sections: Section<Tab>[] = [
    { id: "details", label: "Details", content: editing ? (
      <ContactForm contact={c} onDone={() => { setEditing(false); }} />
    ) : (
      <section className="panel candidate-profile" aria-label="Contact details">
        <dl>
          <dt>Job title</dt><dd>{c.title ?? "—"}</dd>
          <dt>Company</dt><dd><a href={`#/companies/${c.companyId}`}>{c.companyName}</a></dd>
          <dt>Role</dt><dd>{roleLabel(fs, c.role)}</dd>
          <dt>Status</dt><dd>{CONTACT_STATUS[c.status]}</dd>
          <dt>Email check</dt>
          <dd>
            {c.email ? <EmailCheckBadge check={c.emailCheck} detailed /> : "No email"}
            {c.email && canEdit && (
              <>
                {" "}
                <button type="button" className="link" disabled={recheck.isPending} onClick={() => { recheck.mutate(); }}>
                  {recheck.isPending ? "Checking…" : "Check again"}
                </button>
              </>
            )}
          </dd>
          <dt>Notes</dt><dd className="pre">{c.notes ?? "—"}</dd>
        </dl>
        <ErrorMessage error={recheck.error} />
      </section>
    ) },
    { id: "employment", label: "Employment check", content: <EmploymentCheck contact={c} canCheck={canEdit && !editing} /> },
    ...(permissions.includes("emails:read") ? [{ id: "email" as const, label: "Email", content:
      <ContactEmails contactId={c.id} email={c.email} permissions={permissions} writing={writing} onWritingChange={setWriting} /> }] : []),
    { id: "activity", label: "Activity", content: <Timeline compact subjectType="contact" subjectId={c.id} permissions={permissions} /> },
  ];

  return (
    <main className="roomy">
      <a href="#/contacts">← Contacts</a>
      {contact.error && (
        <ErrorMessage error={contact.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      {!editing && (canEdit || canEmail) && (
        <div className="row spread">
          <span />
          <span className="row">
            {canEmail && <button type="button" onClick={() => { setWriting(true); setActive("email"); }}>Email</button>}
            {canEdit && <button type="button" onClick={() => { setEditing(true); setActive("details"); }}>Edit contact</button>}
          </span>
        </div>
      )}
      <ContactBanner contact={c} canEdit={canEdit && !editing} />
      <SectionTabs name="contact" label="Contact sections" sections={sections} active={active} onChange={setActive} />
    </main>
  );
}
