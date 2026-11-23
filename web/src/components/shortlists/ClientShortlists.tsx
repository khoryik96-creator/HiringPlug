// A job's client shortlist links: whether the client opened each, their
// answer and comment for each candidate, and revoking a link. A new link
// starts with the candidates at Submitted (or those chosen on the pipeline).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { clientShortlistsKey } from "../../api/keys";
import { formatWhen } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { ClientLinkForm, type LinkCandidate } from "./ClientLinkForm";

type Link = Schemas["ClientShortlist"];

const STATUS: Record<Schemas["ClientShortlistStatus"], string> = { open: "Open", expired: "Expired", revoked: "Revoked" };
const day = (iso: string) => new Date(iso).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });

/** How many of a link's candidates the client has answered, in words. */
export function answeredText(l: Link): string {
  const yes = l.candidates.filter((c) => c.decision === "approved").length;
  const no = l.candidates.filter((c) => c.decision === "rejected").length;
  if (yes + no === 0) return "No answers yet";
  return [yes > 0 ? `${String(yes)} approved` : "", no > 0 ? `${String(no)} rejected` : "", yes + no < l.candidates.length ? `${String(l.candidates.length - yes - no)} waiting` : ""]
    .filter(Boolean).join(", ");
}

export function ClientShortlists({ jobId, contactName, canEmail, start, onStarted }: {
  jobId: string; contactName?: string | null; canEmail: boolean;
  /** Candidates chosen on the pipeline for a new link. */
  start?: LinkCandidate[] | null; onStarted?: () => void;
}) {
  const links = useQuery({
    queryKey: clientShortlistsKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/client-shortlists", { params: { path: { jobId } } })).items,
  });
  const [making, setMaking] = useState<LinkCandidate[] | null>(null);
  const submitted = useMutation({
    mutationFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/applications", { params: { path: { jobId }, query: { stage: "submitted" } } })).items,
    onSuccess: (apps) => { setMaking(apps.map((a) => ({ applicationId: a.id, candidateId: a.candidateId, name: a.candidateName, stage: a.stage }))); },
  });
  const people = start ?? making;
  const close = () => { setMaking(null); submitted.reset(); onStarted?.(); };
  return (
    <section className="panel client-links" aria-label="Client links" id="client-links">
      <div className="row spread">
        <h3>Client links <span className="count-chip">{links.data?.length ?? 0}</span></h3>
        {!people && <button type="button" disabled={submitted.isPending} onClick={() => { submitted.mutate(); }}>New link for those at Submitted</button>}
      </div>
      <ErrorMessage error={links.error ?? submitted.error} />
      {people && <ClientLinkForm key={people.map((p) => p.applicationId).join()} jobId={jobId} people={people}
        contactName={contactName} canEmail={canEmail} onClose={close} />}
      {links.data?.length === 0 && !people && (
        <p className="muted small">
          No links yet. Send the client a link to the candidates you put forward: they read each CV and approve or reject them,
          and their answers move the candidates here. Choose candidates on the pipeline, or start from those at Submitted.
        </p>
      )}
      {links.data?.map((l) => <LinkCard key={l.id} link={l} jobId={jobId} />)}
    </section>
  );
}

function LinkCard({ link: l, jobId }: { link: Link; jobId: string }) {
  const queryClient = useQueryClient();
  const revoke = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/client-shortlists/{shortlistId}/revoke", { params: { path: { shortlistId: l.id } } })),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: clientShortlistsKey(jobId) }); },
  });
  return (
    <article className={`client-link-card ${l.status}`} aria-label={`Link of ${day(l.createdAt)}`}>
      <div className="row spread">
        <p className="client-link-meta">
          <span className={`badge link-${l.status}`}>{STATUS[l.status]}</span>{" "}
          <strong>{day(l.createdAt)}</strong>{l.createdByName ? ` by ${l.createdByName}` : ""}
          {l.emailedTo ? <span className="muted"> · emailed to {l.emailedTo}</span> : null}
          <span className="muted"> · {l.status === "open" ? `works until ${day(l.expiresAt)}` : l.revokedAt ? `revoked ${day(l.revokedAt)}` : `expired ${day(l.expiresAt)}`}</span>
        </p>
        {l.status === "open" && (
          <button type="button" className="danger-quiet" disabled={revoke.isPending}
            onClick={() => { if (window.confirm("Revoke this link? The client will no longer be able to open it.")) revoke.mutate(); }}>Revoke</button>
        )}
      </div>
      <p className="small">
        {l.openedAt ? <span className="opened">Opened {formatWhen(l.openedAt)}</span> : <span className="muted">Not opened yet</span>}
        {" · "}{answeredText(l)}
      </p>
      <ErrorMessage error={revoke.error} />
      <ul className="client-link-people">
        {l.candidates.map((c) => (
          <li key={c.id} className={c.decision ?? "waiting"}>
            <span className={`answer ${c.decision ?? "waiting"}`}>{c.decision === "approved" ? "✓ Approved" : c.decision === "rejected" ? "✕ Rejected" : "Waiting"}</span>
            <a href={`#/candidates/${c.candidateId}`}>{c.candidateName}</a>
            {c.blind && <span className="muted small"> (blind: shown as {c.label})</span>}
            {c.comment && <span className="client-comment">“{c.comment}”</span>}
          </li>
        ))}
      </ul>
    </article>
  );
}
