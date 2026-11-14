import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { applicationsKey, jobKey } from "../api/keys";
import { Assistant } from "../components/Assistant";
import { ErrorMessage } from "../components/ErrorMessage";
import { Timeline } from "../components/Timeline";
import { JobAds } from "../components/JobAds";
import { MarketMap } from "../components/MarketMap";
import { JdFiles } from "../components/JdFiles";
import { JobForm } from "../components/JobForm";
import { JobRetainer } from "../components/JobRetainer";
import { JobStatusSelect } from "../components/JobStatusSelect";
import { JobSalaryPanel } from "../components/JobSalaryPanel";
import { Owner } from "../components/Owner";
import { PipelineBoard } from "../components/PipelineBoard";
import { PossibleCandidates } from "../components/PossibleCandidates";
import { countryName, formatDay } from "../labels";
import { useFieldSettings } from "../components/useFieldSettings";

/** One job. Render it with key={id} so edit state never carries over. */
export function JobDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const fs = useFieldSettings();
  const queryClient = useQueryClient();
  const job = useQuery({
    queryKey: jobKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}", { params: { path: { jobId: id } } })),
  });
  const j = job.data;
  if (!j) return job.error ? <ErrorMessage error={job.error} /> : <p className="muted">Loading…</p>;

  const facts = [
    [j.location, j.country ? countryName(j.country) : null].filter(Boolean).join(", "),
    j.workType ? fs.label("work_type", j.workType) : null,
    j.workArrangement ? fs.label("work_arrangement", j.workArrangement) : null,
    `${String(j.openings)} opening${j.openings === 1 ? "" : "s"}`,
    `intake ${formatDay(j.intakeDate)}`,
  ].filter(Boolean).join(" · ");
  return (
    <main>
      <a href="#/jobs">← Jobs</a>
      {job.error && (
        <ErrorMessage error={job.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      <div className="row spread">
        <div className="row job-title">
          <h2>{j.title}</h2>
          <JobStatusSelect job={j} canChange={permissions.includes("jobs:update") && !editing} />
        </div>
        {permissions.includes("jobs:update") && !editing && (
          <button type="button" onClick={() => { setEditing(true); }}>Edit job</button>
        )}
      </div>
      <p className="muted">
        <a href={`#/companies/${j.companyId}`}>{j.companyName}</a>
        {j.contactId && <> · contact <a href={`#/contacts/${j.contactId}`}>{j.contactName}</a></>}
        {" · "}
        <Owner what="job" ownerId={j.ownerId} ownerName={j.ownerName} canChange={permissions.includes("jobs:update")}
          save={async (ownerId) => {
            unwrap(await api.PUT("/v1/jobs/{jobId}/owner", { params: { path: { jobId: j.id } }, body: { ownerId } }));
            await queryClient.invalidateQueries({ queryKey: jobKey(j.id) });
            await queryClient.invalidateQueries({ queryKey: applicationsKey }); // who sees its candidates may change
          }} />
      </p>
      {editing && <JobForm job={j} onDone={() => { setEditing(false); }} />}
      {permissions.includes("pipeline:read") && (
        <PipelineBoard jobId={j.id} canUpdate={permissions.includes("pipeline:update")} permissions={permissions}>
          <PossibleCandidates jobId={j.id} permissions={permissions} />
        </PipelineBoard>
      )}
      <Timeline subjectType="job" subjectId={j.id} permissions={permissions} />
      {!editing && (
        <details className="panel fold" aria-label="Job details">
          <summary><span className="fold-title">Job details</span></summary>
          <p>{facts}</p>
          <p className="pre job-description">{j.description ?? <span className="muted">No description yet.</span>}</p>
          <JdFiles jobId={j.id} permissions={permissions} />
        </details>
      )}
      <JobAds job={j} canEdit={permissions.includes("jobs:update")} />
      <MarketMap jobId={j.id} permissions={permissions} />
      <Assistant subjectType="job" subjectId={j.id} permissions={permissions} />
      <JobSalaryPanel job={j} canEdit={permissions.includes("jobs.salary:update")} />
      <JobRetainer jobId={j.id} permissions={permissions} />
    </main>
  );
}
