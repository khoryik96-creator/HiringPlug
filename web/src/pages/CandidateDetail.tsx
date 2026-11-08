import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Candidate, type Permission, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";
import { Assistant } from "../components/Assistant";
import { CandidateApplications } from "../components/CandidateApplications";
import { CandidateBanner } from "../components/CandidateBanner";
import { CandidateCv } from "../components/CandidateCv";
import { CandidateForm } from "../components/CandidateForm";
import { CandidateEmails } from "../components/CandidateEmails";
import { Documents } from "../components/Documents";
import { PrivacyPanel } from "../components/PrivacyPanel";
import { Chips } from "../components/LabelInput";
import { ErrorMessage } from "../components/ErrorMessage";
import { History } from "../components/History";
import { Owner } from "../components/Owner";
import { Timeline } from "../components/Timeline";
import { SalaryPanel } from "../components/SalaryPanel";
import { SimilarCandidates } from "../components/SimilarCandidates";
import { StageMoves } from "../components/StageMoves";
import { WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";

function workAuthorization(c: Candidate): string {
  const wa = c.workAuthorization;
  if (!wa) return "—";
  const status = WORK_AUTH_STATUS[wa.status];
  return wa.status === "expat_eligible" && wa.basis ? `${status}: ${WORK_AUTH_BASIS[wa.basis]}` : status;
}

/**
 * One candidate: profile, salary (if permitted) and documents. Render it with
 * key={id}: edit state belongs to one candidate and must not carry over.
 */
export function CandidateDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const queryClient = useQueryClient();
  const candidate = useQuery({
    queryKey: candidateKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}", { params: { path: { candidateId: id } } })),
  });
  const c = candidate.data;
  if (!c) {
    if (!candidate.error) return <p className="muted">Loading…</p>;
    return (
      <ErrorMessage error={candidate.error}>
        {candidate.error instanceof ApiError && candidate.error.code === "NOT_FOUND" && (
          <p className="muted">They may belong to someone else: your role may see only some candidates. <a href="#/candidates">Back to candidates</a></p>
        )}
      </ErrorMessage>
    );
  }

  return (
    <main className="roomy">
      <a href="#/candidates">← Candidates</a>
      {/* A failed background refresh keeps the page (and any open form) and says so. */}
      {candidate.error && (
        <ErrorMessage error={candidate.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      <div className="row spread">
        <span className="row muted"><Owner what="candidate" ownerId={c.ownerId} ownerName={c.ownerName} canChange={permissions.includes("candidates:update")}
          save={async (ownerId) => {
            unwrap(await api.PUT("/v1/candidates/{candidateId}/owner", { params: { path: { candidateId: c.id } }, body: { ownerId } }));
            await queryClient.invalidateQueries({ queryKey: candidatesKey });
            await queryClient.invalidateQueries({ queryKey: candidateKey(c.id) });
          }} /></span>
        {permissions.includes("candidates:update") && !editing && (
          <span className="row">
            <a className="button" href={`#/candidates/${c.id}/screening`}>Screening call</a>
            <button type="button" onClick={() => { setEditing(true); }}>Edit profile</button>
          </span>
        )}
      </div>
      <CandidateBanner candidate={c} canEdit={permissions.includes("candidates:update")} />
      {/* Notes (and the timeline) right under the card: the first thing read after it. */}
      <Timeline subjectType="candidate" subjectId={c.id} permissions={permissions} />
      <SimilarCandidates candidate={c} canMerge={permissions.includes("candidates:merge")} />
      {editing ? (
        <CandidateForm candidate={c} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel" aria-label="Profile">
          <dl>
            <dt>Email</dt>
            <dd>{c.email ?? "—"}</dd>
            <dt>Phone</dt>
            <dd>{c.phone ?? "—"}</dd>
            <dt>LinkedIn</dt>
            <dd>{c.linkedinUrl ? <a href={c.linkedinUrl} target="_blank" rel="noreferrer noopener">{c.linkedinUrl.replace(/^https:\/\/(www\.)?/, "")}</a> : "—"}</dd>
            <dt>Work authorization</dt>
            <dd>{workAuthorization(c)}</dd>
            <dt>Industry / specialization</dt>
            <dd>{[c.industry, c.specialization].filter(Boolean).join(" / ") || "—"}</dd>
            <dt>Tags</dt>
            <dd><Chips labels={c.tags} /></dd>
          </dl>
        </section>
      )}
      {permissions.includes("documents:read") && <CandidateCv candidateId={c.id} />}
      <History candidate={c} canEdit={permissions.includes("candidates:update")} />
      {permissions.includes("pipeline:read") && <CandidateApplications candidateId={c.id} canUpdate={permissions.includes("pipeline:update")} permissions={permissions} />}
      {permissions.includes("pipeline:read") && <StageMoves candidateId={c.id} title="Stage history" limit={50} />}
      <Assistant subjectType="candidate" subjectId={c.id} permissions={permissions} />
      <SalaryPanel candidate={c} canEdit={permissions.includes("candidates.compensation:update")} />
      {permissions.includes("emails:read") && <CandidateEmails candidateId={c.id} permissions={permissions} />}
      <Documents candidate={c} canUpload={permissions.includes("documents:create")} canEdit={permissions.includes("candidates:update")} permissions={permissions} />
      <PrivacyPanel candidateId={c.id} candidateName={c.fullName} permissions={permissions} />
    </main>
  );
}
