import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Permission, unwrap } from "../api/client";
import { CandidateForm } from "../components/CandidateForm";
import { Documents } from "../components/Documents";
import { ErrorMessage } from "../components/ErrorMessage";
import { SalaryPanel } from "../components/SalaryPanel";

const BASIS: Record<string, string> = {
  rp_t: "Residence Pass-Talent",
  permanent_resident: "Permanent resident",
  spouse_visa: "Spouse visa",
  other: "Other pass",
};

function workAuthorization(c: Candidate): string {
  const wa = c.workAuthorization;
  if (!wa) return "—";
  switch (wa.status) {
    case "local":
      return "Local";
    case "requires_visa":
      return "Requires a visa";
    case "expat_eligible":
      return `Expat, can work here (${BASIS[wa.basis ?? "other"] ?? "pass"})`;
  }
}

/** One candidate: profile, salary (if permitted) and documents. */
export function CandidateDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const candidate = useQuery({
    queryKey: ["candidate", id],
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}", { params: { path: { candidateId: id } } })),
  });
  if (candidate.error) return <ErrorMessage error={candidate.error} />;
  const c = candidate.data;
  if (!c) return <p className="muted">Loading…</p>;

  const notice = c.noticePeriod
    ? c.noticePeriod.value === 0
      ? "Immediate"
      : `${c.noticePeriod.value} ${c.noticePeriod.unit}${c.noticePeriod.value === 1 ? "" : "s"}`
    : "—";
  return (
    <main>
      <a href="#/candidates">← Candidates</a>
      <div className="row spread">
        <h2>{c.fullName}</h2>
        {permissions.includes("candidates:update") && !editing && (
          <button type="button" onClick={() => { setEditing(true); }}>Edit profile</button>
        )}
      </div>
      <p className="muted">{[c.currentTitle, c.currentCompany, c.location].filter(Boolean).join(" · ")}</p>
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
          <dd>{c.nationality ?? "—"}</dd>
          <dt>Work authorization</dt>
          <dd>{workAuthorization(c)}</dd>
          <dt>Notice period</dt>
          <dd>{notice}</dd>
          <dt>Industry / specialization</dt>
          <dd>{[c.industry, c.specialization].filter(Boolean).join(" / ") || "—"}</dd>
          <dt>Source</dt>
          <dd>{c.source?.replace("_", " ") ?? "—"}</dd>
        </dl>
      </section>
      )}
      <SalaryPanel candidate={c} canEdit={permissions.includes("candidates.compensation:update")} />
      <Documents candidateId={c.id} canUpload={permissions.includes("documents:create")} />
    </main>
  );
}
