import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { contactKey } from "../api/keys";
import { ContactForm } from "../components/ContactForm";
import { EmailCheckBadge } from "../components/EmailCheckBadge";
import { EmploymentCheck } from "../components/EmploymentCheck";
import { ErrorMessage } from "../components/ErrorMessage";
import { Timeline } from "../components/Timeline";
import { CONTACT_STATUS } from "../labels";

/** One contact. Render it with key={id} so edit state never carries over. */
export function ContactDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
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

  return (
    <main>
      <a href="#/contacts">← Contacts</a>
      {contact.error && (
        <ErrorMessage error={contact.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      <div className="row spread">
        <h2>{c.fullName}</h2>
        {canEdit && !editing && <button type="button" onClick={() => { setEditing(true); }}>Edit contact</button>}
      </div>
      <p className="muted">
        {c.title ? `${c.title} at ` : ""}<a href={`#/companies/${c.companyId}`}>{c.companyName}</a>
        {c.status === "left" && ` · ${CONTACT_STATUS.left}`}
      </p>
      {editing ? (
        <ContactForm contact={c} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel" aria-label="Contact details">
          <dl>
            <dt>Email</dt>
            <dd>
              {c.email ?? "—"} <EmailCheckBadge check={c.emailCheck} detailed />
              {c.email && canEdit && (
                <>
                  {" "}
                  <button type="button" className="link" disabled={recheck.isPending} onClick={() => { recheck.mutate(); }}>
                    {recheck.isPending ? "Checking…" : "Check again"}
                  </button>
                </>
              )}
            </dd>
            <dt>Office phone</dt>
            <dd>{c.phone ?? "—"}</dd>
            <dt>Mobile</dt>
            <dd>{c.mobile ?? "—"}</dd>
            <dt>LinkedIn</dt>
            <dd>{c.linkedinUrl ? <a href={c.linkedinUrl} target="_blank" rel="noreferrer">{c.linkedinUrl}</a> : "—"}</dd>
            <dt>Notes</dt>
            <dd className="pre">{c.notes ?? "—"}</dd>
          </dl>
          <ErrorMessage error={recheck.error} />
        </section>
      )}
      {!editing && <EmploymentCheck contact={c} canCheck={canEdit} />}
      <Timeline subjectType="contact" subjectId={c.id} permissions={permissions} />
    </main>
  );
}
