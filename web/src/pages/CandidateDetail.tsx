import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Permission, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { CandidateApplications } from "../components/CandidateApplications";
import { CandidateForm } from "../components/CandidateForm";
import { Documents } from "../components/Documents";
import { Chips } from "../components/LabelInput";
import { ErrorMessage } from "../components/ErrorMessage";
import { History } from "../components/History";
import { Timeline } from "../components/Timeline";
import { SalaryPanel } from "../components/SalaryPanel";
import { SimilarCandidates } from "../components/SimilarCandidates";
import { CANDIDATE_SOURCE, countryName, formatNotice, WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";

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
  const candidate = useQuery({
    queryKey: candidateKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}", { params: { path: { candidateId: id } } })),
  });
  const c = candidate.data;
  if (!c) return candidate.error ? <ErrorMessage error={candidate.error} /> : <p className="muted">Loading…</p>;

  return (
    <main>
      <a href="#/candidates">← Candidates</a>
      {/* A failed background refresh keeps the page (and any open form) and says so. */}
      {candidate.error && (
        <ErrorMessage error={candidate.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      <div className="row spread">
        <h2>{c.fullName}</h2>
        {permissions.includes("candidates:update") && !editing && (
          <button type="button" onClick={() => { setEditing(true); }}>Edit profile</button>
        )}
      </div>
      <p className="muted">{[c.currentTitle, c.currentCompany, c.location].filter(Boolean).join(" · ")}</p>
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
            <dt>Nationality</dt>
            <dd>{c.nationality ? countryName(c.nationality) : "—"}</dd>
            <dt>Work authorization</dt>
            <dd>{workAuthorization(c)}</dd>
            <dt>Notice period</dt>
            <dd>{formatNotice(c.noticePeriod)}</dd>
            <dt>Industry / specialization</dt>
            <dd>{[c.industry, c.specialization].filter(Boolean).join(" / ") || "—"}</dd>
            <dt>Source</dt>
            <dd>{c.source ? CANDIDATE_SOURCE[c.source] : "—"}</dd>
            <dt>Skills</dt>
            <dd><Chips labels={c.skills} /></dd>
            <dt>Tags</dt>
            <dd><Chips labels={c.tags} /></dd>
          </dl>
        </section>
      )}
      <History candidate={c} canEdit={permissions.includes("candidates:update")} />
      {permissions.includes("pipeline:read") && <CandidateApplications candidateId={c.id} canUpdate={permissions.includes("pipeline:update")} permissions={permissions} />}
      <SalaryPanel candidate={c} canEdit={permissions.includes("candidates.compensation:update")} />
      <Documents candidate={c} canUpload={permissions.includes("documents:create")} canEdit={permissions.includes("candidates:update")} />
      <Timeline subjectType="candidate" subjectId={c.id} permissions={permissions} />
    </main>
  );
}
