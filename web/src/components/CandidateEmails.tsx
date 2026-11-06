import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { candidateApplicationsKey, candidateEmailsKey } from "../api/keys";
import { EMAIL_STATUS, formatWhen } from "../labels";
import { CandidateBookings } from "./CandidateBookings";
import { EmailComposer } from "./EmailComposer";
import { ErrorMessage } from "./ErrorMessage";

/** Emails sent about a candidate, newest first, and writing a new one. */
export function CandidateEmails({ candidateId, permissions }: { candidateId: string; permissions: Permission[] }) {
  const [writing, setWriting] = useState(false);
  const canSend = permissions.includes("emails:send");
  const list = useQuery({
    queryKey: candidateEmailsKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/emails", { params: { query: { candidateId } } })).items,
    // Sending happens in the background: look again while any is on its way.
    refetchInterval: (q) => (q.state.data?.some((e) => e.status === "queued") ? 3000 : false),
  });
  const apps = useQuery({
    queryKey: candidateApplicationsKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/applications", { params: { path: { candidateId } } })),
    enabled: writing && permissions.includes("pipeline:read"),
  });
  return (
    <section className="panel" aria-label="Emails">
      <div className="row spread">
        <h3>Emails</h3>
        {canSend && !writing && <button type="button" onClick={() => { setWriting(true); }}>Write email</button>}
      </div>
      {writing && (
        <EmailComposer
          candidateId={candidateId}
          applications={apps.data?.items.map((a) => ({ id: a.id, label: `${a.jobTitle} at ${a.companyName}` }))}
          canManageTemplates={permissions.includes("emails.templates:manage")}
          canBook={permissions.includes("scheduling:use")}
          onDone={() => { setWriting(false); }}
        />
      )}
      {permissions.includes("scheduling:use") && <CandidateBookings candidateId={candidateId} />}
      <ErrorMessage error={list.error} />
      {list.data?.length === 0 && <p className="muted">No emails sent from HiringPlug yet.</p>}
      {list.data?.map((e) => (
        <details key={e.id} className="email">
          <summary>
            <strong>{e.subject}</strong> <span className={`badge ${e.status}`}>{EMAIL_STATUS[e.status]}</span>{" "}
            <span className="muted">to {e.to.join(", ")} · {e.sentByName} · {formatWhen(e.sentAt ?? e.createdAt)}</span>
          </summary>
          {e.error && <p className="warning">Why: {e.error}</p>}
          {e.cc.length > 0 && <p className="muted">Cc {e.cc.join(", ")}</p>}
          {e.bcc.length > 0 && <p className="muted">Bcc {e.bcc.join(", ")}</p>}
          {e.attachments.length > 0 && <p className="muted">Attached: {e.attachments.map((a) => a.fileName).join(", ")}</p>}
          {e.salary && <p className="muted small">Shows salary.</p>}
          <p className="email-body">{e.body}</p>
        </details>
      ))}
    </section>
  );
}
