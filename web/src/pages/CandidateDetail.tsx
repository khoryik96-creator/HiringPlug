import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Permission, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";
import { Assistant } from "../components/Assistant";
import { CandidateApplications } from "../components/CandidateApplications";
import { CandidateBanner } from "../components/CandidateBanner";
import { CandidateTabs, type CandidateTab, type CandidateSection } from "../components/CandidateTabs";
import { WorkAuthorizationPicker } from "../components/WorkAuthorizationPicker";
import { hashParam } from "../hashQuery";
import { CandidateForm } from "../components/CandidateForm";
import { CandidateEmails } from "../components/CandidateEmails";
import { EditIcon, IconActions, IconButton, IconLink, IconSeparator, MailIcon, PhoneIcon, ReferenceIcon, SideBySideIcon, TrashIcon } from "../components/ActionIcons";
import { DeleteRecord } from "../components/DeleteRecord";
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
import { CvBeside, useCvBeside } from "../components/CvBeside";
import { useFieldSettings } from "../components/useFieldSettings";
/**
 * One candidate: profile, salary (if permitted) and documents. Render it with
 * key={id}: edit state belongs to one candidate and must not carry over.
 */
export function CandidateDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const [editingSalary, setEditingSalary] = useState(false);
  const [active, setActive] = useState<CandidateTab>(() =>
    (hashParam("hl") || hashParam("tab") === "cv") && permissions.includes("documents:read") ? "cv" : "details",
  );
  const [writing, setWriting] = useState(false);
  const [besideOn, setBeside] = useCvBeside();
  const fs = useFieldSettings();
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
  const canSeeCv = permissions.includes("documents:read");
  // The CV & files tab already shows the CV: not twice.
  const beside = canSeeCv && besideOn && active !== "cv";
  const sections: CandidateSection[] = [
    { id: "details", label: "Details", content: <>
      <SimilarCandidates candidate={c} canMerge={permissions.includes("candidates:merge")} />
      {editing ? (
        <CandidateForm candidate={c} canMerge={permissions.includes("candidates:merge")} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel candidate-profile one-column" aria-label="Profile">
          {/* Email, phone and LinkedIn are on the card above (click to copy). */}
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
            <dt>Source</dt>
            <dd>{c.source ? fs.label("candidate_source", c.source) : "—"}</dd>
          </dl>
        </section>
      )}
      <History candidate={c} canEdit={permissions.includes("candidates:update")} />
      <PrivacyPanel candidateId={c.id} candidateName={c.fullName} permissions={permissions} />
    </> },
    ...(permissions.includes("documents:read") ? [{ id: "cv" as const, label: "CV & files", content:
      <Documents candidate={c} canUpload={permissions.includes("documents:create")} canEdit={permissions.includes("candidates:update")} permissions={permissions} /> }] : []),
    ...(permissions.includes("pipeline:read") ? [{ id: "jobs" as const, label: "Jobs", content: <>
      <CandidateApplications candidateId={c.id} canUpdate={permissions.includes("pipeline:update")} permissions={permissions} />
      <StageMoves candidateId={c.id} title="Stage history" limit={50} />
    </> }] : []),
    { id: "activity", label: "Activity", content: <Timeline compact subjectType="candidate" subjectId={c.id} permissions={permissions} /> },
    { id: "salary", label: "Salary", content: <SalaryPanel candidate={c} canEdit={permissions.includes("candidates.compensation:update")}
      editing={editingSalary} onEditingChange={setEditingSalary} /> },
    ...(permissions.includes("emails:read") ? [{ id: "email" as const, label: "Email", content:
      <CandidateEmails candidateId={c.id} email={c.email} permissions={permissions} writing={writing} onWritingChange={setWriting} /> }] : []),
    // Beside Email; kept mounted, as it may be docked on the right whatever tab is open.
    ...(permissions.includes("assistant:use") ? [{ id: "assistant" as const, label: "AI assistant", keep: true, content:
      <Assistant subjectType="candidate" subjectId={c.id} permissions={permissions} /> }] : []),
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
          <IconActions label="Candidate actions">
            {permissions.includes("emails:read") && permissions.includes("emails:send") &&
              <IconButton label="Email" icon={<MailIcon />} onClick={() => { setWriting(true); setActive("email"); }} />}
            <IconLink label="Screening call" icon={<PhoneIcon />} href={`#/candidates/${c.id}/screening`} />
            {permissions.includes("notes:create") && <IconLink label="Reference check" icon={<ReferenceIcon />} href={`#/candidates/${c.id}/reference-check`} />}
            <IconButton label="Edit profile" icon={<EditIcon />} onClick={() => { setEditing(true); setActive("details"); }} />
            {permissions.includes("bin:manage") && <><IconSeparator /><DeleteRecord kind="candidate" id={c.id} icon={<TrashIcon />} /></>}
          </IconActions>
        )}
      </div>
      <CandidateBanner candidate={c} canEdit={permissions.includes("candidates:update")} canEditStatus={permissions.includes("candidates.status:update")}
        onAdd={() => { setEditing(true); setActive("details"); }}
        canEditSalary={permissions.includes("candidates.compensation:update")}
        onSalary={() => { setEditingSalary(true); setActive("salary"); }} />
      <div className={beside ? "candidate-workspace with-cv" : "candidate-workspace"}>
        {canSeeCv && active !== "cv" && <IconButton label="CV beside" icon={<SideBySideIcon />} className="cv-beside-toggle"
          pressed={beside} onClick={() => { setBeside(!beside); }} />}
        <div className="candidate-main">
        <CandidateTabs sections={sections} active={active} onChange={setActive} />
        </div>
        {beside && <CvBeside candidateId={c.id} />}
      </div>
    </main>
  );
}
