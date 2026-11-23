import { SimilarContacts } from "../components/SimilarRecords";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { EditIcon, IconActions, IconButton, IconSeparator, MailCheckIcon, MailIcon, RefreshIcon, TrashIcon } from "../components/ActionIcons";
import { DeleteRecord } from "../components/DeleteRecord";

import { api, type Permission, unwrap } from "../api/client";
import { contactKey, contactsKey } from "../api/keys";
import { ContactBanner } from "../components/ContactBanner";
import { ContactEmails } from "../components/ContactEmails";
import { ContactForm } from "../components/ContactForm";
import { EmailCheckBadge } from "../components/EmailCheckBadge";
import { EmailSuggestion } from "../components/EmailSuggestion";
import { EmploymentCheck } from "../components/EmploymentCheck";
import { JobsNeedingContact } from "../components/FollowLeaver";
import { LinkedinCheckResult, LinkedinChecks } from "../components/LinkedinChecks";
import { MailboxBadge } from "../components/MailboxBadge";
import { ErrorMessage } from "../components/ErrorMessage";
import { useTabName } from "../components/openRecords";
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
    // Follow an employment check still being read, or an email being verified.
    refetchInterval: (q) => (q.state.data?.employment?.status === "reading" || q.state.data?.mailboxCheck?.status === "checking" ? 2000 : false),
  });
  const recheck = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/contacts/{contactId}/email-check", { params: { path: { contactId: id } } })),
    onSuccess: (updated) => { queryClient.setQueryData(contactKey(id), updated); },
  });
  const verify = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/contacts/{contactId}/mailbox-check", { params: { path: { contactId: id } } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: contactKey(id) });
      await queryClient.invalidateQueries({ queryKey: contactsKey });
    },
  });
  useTabName(`#/contacts/${id}`, contact.data?.fullName); // its tab, by the contact's name
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
                <IconButton label={recheck.isPending ? "Checking…" : "Check again"} icon={<RefreshIcon />}
                  disabled={recheck.isPending} onClick={() => { recheck.mutate(); }} />
                {recheck.isPending && <span role="status" className="muted small">Checking…</span>}
              </>
            )}
          </dd>
          <dt>Mailbox</dt>
          <dd>{c.mailboxCheck ? <MailboxBadge check={c.mailboxCheck} detailed /> : <span className="muted">Not verified yet</span>}</dd>
          <dt>Notes</dt><dd className="pre">{c.notes ?? "—"}</dd>
        </dl>
        <ErrorMessage error={recheck.error} />
      </section>
    ) },
    { id: "employment", label: "Employment check", content: (
      <>
        {canEdit && !editing && c.linkedinUrl && (
          <section className="panel" aria-label="LinkedIn check">
            <h3>On LinkedIn</h3>
            {c.linkedinCheck && <LinkedinCheckResult check={c.linkedinCheck} />}
            <LinkedinChecks contactId={c.id} label="Check on LinkedIn now" />
          </section>
        )}
        <EmploymentCheck contact={c} canCheck={canEdit && !editing} />
      </>
    ) },
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
          <IconActions label="Contact actions">
            {canEdit && c.email && (
              <IconButton label={c.mailboxCheck?.status === "checking" ? "Verifying…" : "Verify email"} icon={<MailCheckIcon />}
                tip={c.mailboxCheck?.status === "checking" ? "Verifying…" : "Verify email (asks their mail server; no email is sent)"}
                disabled={verify.isPending || c.mailboxCheck?.status === "checking"}
                onClick={() => { setActive("details"); verify.mutate(); }} />
            )}
            {canEmail && <IconButton label="Email" icon={<MailIcon />} onClick={() => { setWriting(true); setActive("email"); }} />}
            {canEdit && <IconButton label="Edit contact" icon={<EditIcon />} onClick={() => { setEditing(true); setActive("details"); }} />}
            {permissions.includes("bin:manage") && <><IconSeparator /><DeleteRecord kind="contact" id={c.id} icon={<TrashIcon />} /></>}
          </IconActions>
        </div>
      )}
      <ErrorMessage error={verify.error} />
      <ContactBanner contact={c} canEdit={canEdit && !editing} canEditRole={permissions.includes("contacts.role:update")} />
      <SimilarContacts contact={c} canMerge={permissions.includes("contacts:merge")} />
      {c.status === "left" && <JobsNeedingContact contactId={c.id} />}
      {c.status === "active" && <EmailSuggestion contact={c} canEdit={canEdit && !editing} />}
      <SectionTabs name="contact" label="Contact sections" sections={sections} active={active} onChange={setActive} />
    </main>
  );
}
