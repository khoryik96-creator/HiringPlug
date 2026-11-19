// Emails to one of a client's contacts, from their page: as on a candidate's
// page, with templates and fields filled from the contact ([First name] is
// theirs) and their company.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { contactEmailsKey } from "../api/keys";
import { IconButton, MailIcon } from "./ActionIcons";
import { SentEmails } from "./CandidateEmails";
import { EmailComposer } from "./EmailComposer";
import { OutlookProfileMail } from "./outlook/OutlookProfileMail";
import { ErrorMessage } from "./ErrorMessage";

export function ContactEmails({ contactId, email, permissions, writing: controlledWriting, onWritingChange }: {
  contactId: string; email: string | null; permissions: Permission[];
  /** With onWritingChange, the page opens the composer (its Email button). */
  writing?: boolean; onWritingChange?: (writing: boolean) => void;
}) {
  const [ownWriting, setOwnWriting] = useState(false);
  const writing = controlledWriting ?? ownWriting;
  const setWriting = onWritingChange ?? setOwnWriting;
  const list = useQuery({
    queryKey: contactEmailsKey(contactId),
    queryFn: async () => unwrap(await api.GET("/v1/emails", { params: { query: { contactId } } })).items,
    refetchInterval: (q) => (q.state.data?.some((e) => e.status === "queued") ? 3000 : false),
  });
  return (
    <section className="panel" aria-label="Emails">
      <div className="row spread">
        <h3>Emails</h3>
        {permissions.includes("emails:send") && !writing && <IconButton label="Write email" icon={<MailIcon />} className="icon-outline"
          onClick={() => { setWriting(true); }} />}
      </div>
      {writing && (
        <EmailComposer permissions={permissions} contactId={contactId} defaultTo={email ?? ""}
          canManageTemplates={permissions.includes("emails.templates:manage")}
          canCreateTemplates={permissions.includes("emails.templates:create")}
          onDone={() => { setWriting(false); }} />
      )}
      <ErrorMessage error={list.error} />
      <SentEmails emails={list.data} />
      {permissions.includes("emails:send") && permissions.includes("contacts:read") && <OutlookProfileMail profile={{ contactId }} address={email} />}
    </section>
  );
}
