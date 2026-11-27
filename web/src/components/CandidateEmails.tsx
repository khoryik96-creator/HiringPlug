import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { type Permission, type Schemas } from "../api/client";
import { loadApplications } from "../api/applications";
import { candidateApplicationsKey, candidateEmailsKey } from "../api/keys";
import { EMAIL_STATUS, formatWhen } from "../labels";
import { IconButton, MailIcon } from "./ActionIcons";
import { CandidateBookings } from "./CandidateBookings";
import { EmailComposer } from "./EmailComposer";
import { OutlookProfileMail } from "./outlook/OutlookProfileMail";
import { ErrorMessage } from "./ErrorMessage";
import { useSentEmails } from "./useSentEmails";

/** Emails sent about a candidate, newest first, and writing a new one. */
export function CandidateEmails({ candidateId, email, permissions, writing: controlledWriting, onWritingChange }: { candidateId: string; email?:string|null;permissions: Permission[]; writing?: boolean; onWritingChange?: (writing: boolean) => void }) {
  const [ownWriting, setOwnWriting] = useState(false);
  const writing = controlledWriting ?? ownWriting;
  const setWriting = onWritingChange ?? setOwnWriting;
  const canSend = permissions.includes("emails:send");
  const list = useSentEmails(candidateEmailsKey(candidateId), { candidateId });
  const apps = useQuery({
    queryKey: candidateApplicationsKey(candidateId),
    queryFn: ({ signal }) => loadApplications("candidate", candidateId, signal),
    enabled: writing && permissions.includes("pipeline:read"),
  });
  return (
    <section className="panel" aria-label="Emails">
      <div className="row spread">
        <h3>Emails</h3>
        {canSend && !writing && <IconButton label="Write email" icon={<MailIcon />} onClick={() => { setWriting(true); }} />}
      </div>
      {writing && (
        <EmailComposer permissions={permissions}
          candidateId={candidateId} defaultTo={email ?? ""}
          applications={apps.data?.items.map((a) => ({ id: a.id, label: `${a.jobTitle} at ${a.companyName}` }))}
          canManageTemplates={permissions.includes("emails.templates:manage")}
          canCreateTemplates={permissions.includes("emails.templates:create")}
          canBook={permissions.includes("scheduling:use")}
          onDone={() => { setWriting(false); }}
        />
      )}
      {permissions.includes("scheduling:use") && <CandidateBookings candidateId={candidateId} />}
      <ErrorMessage error={list.error} />
      <SentEmails emails={list.data} />
      {canSend && permissions.includes("candidates:read") && <OutlookProfileMail profile={{ candidateId }} address={email} />}
    </section>
  );
}

/** Emails sent, newest first: subject, status, to whom, by whom, when; opened to read. */
export function SentEmails({ emails }: { emails: Schemas["Email"][] | undefined }) {
  return (
    <>
      {emails?.length === 0 && <p className="muted">No emails sent from HyPlug yet.</p>}
      {emails?.map((e) => (
        <details key={e.id} className="email">
          <summary>
            <strong>{e.subject}</strong> <span className={`badge ${e.status}`}>{e.status==="sent" && e.deliveryMode==="test" ? "Captured in test inbox" : e.status==="sent" && e.deliveryMode ? "Accepted by mail server" : EMAIL_STATUS[e.status]}</span>{" "}
            {e.bouncedTo.length > 0 && <><span className="badge failed">Bounced</span>{" "}</>}
            <span className="muted">to {e.to.join(", ")} · {e.sentByName} · {formatWhen(e.sentAt ?? e.createdAt)}</span>
          </summary>
          {e.deliveryMode==="test" && <p className="warning small">Local test email; not delivered to a real mailbox.</p>}
          {e.error && <p className="warning">Why: {e.error}</p>}
          {e.bouncedTo.length > 0 && <p className="warning">Bounced back from {e.bouncedTo.join(", ")}: the mailbox does not exist.</p>}
          {e.cc.length > 0 && <p className="muted">Cc {e.cc.join(", ")}</p>}
          {e.bcc.length > 0 && <p className="muted">Bcc {e.bcc.join(", ")}</p>}
          {e.attachments.length > 0 && <p className="muted">Attached: {e.attachments.map((a) => a.fileName).join(", ")}</p>}
          {e.salary && <p className="muted small">Shows salary.</p>}
          <p className="email-body">{e.body}</p>
        </details>
      ))}
    </>
  );
}
