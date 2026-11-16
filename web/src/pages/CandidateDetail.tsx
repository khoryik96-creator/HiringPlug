import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Permission, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";
import { Assistant } from "../components/Assistant";
import { CandidateApplications } from "../components/CandidateApplications";
import { CandidateBanner } from "../components/CandidateBanner";
import { CopyButton } from "../components/CopyButton";
import { CandidateTabs, type CandidateTab, type CandidateSection } from "../components/CandidateTabs";
import { WorkAuthorizationPicker } from "../components/WorkAuthorizationPicker";
import { hashParam } from "../hashQuery";
import { CandidateForm } from "../components/CandidateForm";
import { CandidateEmails } from "../components/CandidateEmails";
import { Documents } from "../components/Documents";
import { PrivacyPanel } from "../components/PrivacyPanel";
import { Chips } from "../components/LabelInput";
import { ErrorMessage } from "../components/ErrorMessage";
import { QuickIndustries, QuickSpecialization, QuickWords } from "../components/ProfileQuickEdit";
import { History } from "../components/History";
import { Owner } from "../components/Owner";
import { Timeline } from "../components/Timeline";
import { SalaryPanel } from "../components/SalaryPanel";
import { SimilarCandidates } from "../components/SimilarCandidates";
import { StageMoves } from "../components/StageMoves";
/**
 * One candidate: profile, salary (if permitted) and documents. Render it with
 * key={id}: edit state belongs to one candidate and must not carry over.
 */
export function CandidateDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const [editingSalary, setEditingSalary] = useState(false);
  const [active, setActive] = useState<CandidateTab>(hashParam("hl") && permissions.includes("documents:read") ? "cv" : "details");
  const [writing, setWriting] = useState(false);
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

  const canUpdate = permissions.includes("candidates:update");
  const sections: CandidateSection[] = [
    { id: "details", label: "Details", content: <>
      <SimilarCandidates candidate={c} canMerge={permissions.includes("candidates:merge")} />
      {editing ? (
        <CandidateForm candidate={c} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel candidate-profile one-column" aria-label="Profile">
          {/* Email, phone and LinkedIn are in the Contact box beside the tabs. */}
          <dl>
            <dt>Work authorization</dt>
            <dd><WorkAuthorizationPicker candidate={c} canEdit={canUpdate} /></dd>
            {/* One column: industries and tags under the specialization. */}
            {/* Changed right here, saved at once (no Edit profile). */}
            <dt>Specialization</dt><dd><QuickSpecialization candidate={c} canEdit={canUpdate} /></dd>
            <dt>Industries</dt><dd><QuickIndustries candidate={c} canEdit={canUpdate} /></dd>
            <dt>IT skills</dt><dd><QuickWords candidate={c} canEdit={canUpdate} field="skills" /></dd>
            <dt>Certifications</dt><dd><QuickWords candidate={c} canEdit={canUpdate} field="certifications" /></dd>
            <dt>Tags</dt>
            <dd><Chips labels={c.tags} /></dd>
          </dl>
        </section>
      )}
      <History candidate={c} canEdit={permissions.includes("candidates:update")} />
      <PrivacyPanel candidateId={c.id} candidateName={c.fullName} permissions={permissions} />
    </> },
    ...(permissions.includes("documents:read") ? [{ id: "cv" as const, label: "CV & files", content:
      <Documents workspace candidate={c} canUpload={permissions.includes("documents:create")} canEdit={permissions.includes("candidates:update")} permissions={permissions} /> }] : []),
    ...(permissions.includes("pipeline:read") ? [{ id: "jobs" as const, label: "Jobs", content: <>
      <CandidateApplications candidateId={c.id} canUpdate={permissions.includes("pipeline:update")} permissions={permissions} />
      <StageMoves candidateId={c.id} title="Stage history" limit={50} />
    </> }] : []),
    { id: "activity", label: "Activity", content: <Timeline compact subjectType="candidate" subjectId={c.id} permissions={permissions} /> },
    { id: "salary", label: "Salary", content: <SalaryPanel candidate={c} canEdit={permissions.includes("candidates.compensation:update")}
      editing={editingSalary} onEditingChange={setEditingSalary} /> },
    ...(permissions.includes("emails:read") ? [{ id: "email" as const, label: "Email", content:
      <CandidateEmails candidateId={c.id} email={c.email} permissions={permissions} writing={writing} onWritingChange={setWriting} /> }] : []),
  ];

  return (
    <main className="roomy">
      <a href="#/candidates">← Candidates</a>
      {hashParam("screeningSalary") === "kept" && <p role="status" className="panel muted">Call saved. A later call already set the salary, so the profile keeps that salary.</p>}
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
          <span className="row candidate-actions">
            {permissions.includes("emails:read") && permissions.includes("emails:send") && <button type="button" onClick={() => { setWriting(true); setActive("email"); }}>Email</button>}
            <a className="button" href={`#/candidates/${c.id}/screening`}>Screening call</a>
            {permissions.includes("notes:create") && <a className="button" href={`#/candidates/${c.id}/reference-check`}>Reference check</a>}
            <button type="button" onClick={() => { setEditing(true); setActive("details"); }}>Edit profile</button>
          </span>
        )}
      </div>
      <CandidateBanner candidate={c} canEdit={permissions.includes("candidates:update")}
        onAdd={() => { setEditing(true); setActive("details"); }}
        canEditSalary={permissions.includes("candidates.compensation:update")}
        onSalary={() => { setEditingSalary(true); setActive("salary"); }} />
      <div className="candidate-workspace">
        <aside className="candidate-contact panel" aria-label="Contact summary">
          <h3>Contact</h3>
          <dl><dt>Email</dt><dd className="copyable">{c.email ? <><a href={`mailto:${c.email}`} title={c.email}>{c.email}</a><CopyButton text={c.email} what="email" /></> : "—"}</dd>
          <dt>Phone</dt><dd className="copyable">{c.phone ? <><a href={`tel:${c.phone}`}>{c.phone}</a><CopyButton text={c.phone} what="phone number" /></> : "—"}</dd>
          <dt>LinkedIn</dt><dd>{c.linkedinUrl ? <a href={c.linkedinUrl} target="_blank" rel="noreferrer noopener" title={c.linkedinUrl}>LinkedIn profile</a> : "—"}</dd></dl>
        </aside>
        <CandidateTabs sections={sections} active={active} onChange={setActive} />
      </div>
      {permissions.includes("assistant:use") && <details className="candidate-assistant"><summary>AI assistant</summary>
        <Assistant subjectType="candidate" subjectId={c.id} permissions={permissions} />
      </details>}
    </main>
  );
}
