// Emails to one of a client's contacts, from their page: as on a candidate's
// page, with templates and fields filled from the contact ([First name] is
// theirs) and their company.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { contactEmailsKey } from "../api/keys";
import { SentEmails } from "./CandidateEmails";
import { EmailComposer } from "./EmailComposer";
import { ErrorMessage } from "./ErrorMessage";

export function ContactEmails({ contactId, email, permissions }: { contactId: string; email: string | null; permissions: Permission[] }) {
  const [writing, setWriting] = useState(false);
  const list = useQuery({
    queryKey: contactEmailsKey(contactId),
    queryFn: async () => unwrap(await api.GET("/v1/emails", { params: { query: { contactId } } })).items,
    refetchInterval: (q) => (q.state.data?.some((e) => e.status === "queued") ? 3000 : false),
  });
  return (
    <section className="panel" aria-label="Emails">
      <div className="row spread">
        <h3>Emails</h3>
        {permissions.includes("emails:send") && !writing && <button type="button" onClick={() => { setWriting(true); }}>Write email</button>}
      </div>
      {writing && (
        <EmailComposer permissions={permissions} contactId={contactId} defaultTo={email ?? ""}
          canManageTemplates={permissions.includes("emails.templates:manage")}
          canCreateTemplates={permissions.includes("emails.templates:create")}
          onDone={() => { setWriting(false); }} />
      )}
      <ErrorMessage error={list.error} />
      <SentEmails emails={list.data} />
    </section>
  );
}
