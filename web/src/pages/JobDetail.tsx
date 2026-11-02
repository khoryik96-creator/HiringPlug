import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { jobKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { Timeline } from "../components/Timeline";
import { JobForm } from "../components/JobForm";
import { JobSalaryPanel } from "../components/JobSalaryPanel";
import { PipelineBoard } from "../components/PipelineBoard";
import { JOB_STATUS, WORK_ARRANGEMENT, WORK_TYPE } from "../labels";

/** One job. Render it with key={id} so edit state never carries over. */
export function JobDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const job = useQuery({
    queryKey: jobKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}", { params: { path: { jobId: id } } })),
  });
  const j = job.data;
  if (!j) return job.error ? <ErrorMessage error={job.error} /> : <p className="muted">Loading…</p>;

  const facts = [
    j.location,
    j.workType ? WORK_TYPE[j.workType] : null,
    j.workArrangement ? WORK_ARRANGEMENT[j.workArrangement] : null,
    `${String(j.openings)} opening${j.openings === 1 ? "" : "s"}`,
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
        <h2>{j.title} <span className={`badge ${j.status}`}>{JOB_STATUS[j.status]}</span></h2>
        {permissions.includes("jobs:update") && !editing && (
          <button type="button" onClick={() => { setEditing(true); }}>Edit job</button>
        )}
      </div>
      <p className="muted">
        <a href={`#/companies/${j.companyId}`}>{j.companyName}</a>
        {j.contactId && <> · contact <a href={`#/contacts/${j.contactId}`}>{j.contactName}</a></>}
      </p>
      {editing ? (
        <JobForm job={j} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel" aria-label="Job details">
          <p>{facts}</p>
          <p className="pre">{j.description ?? <span className="muted">No description yet.</span>}</p>
        </section>
      )}
      {permissions.includes("pipeline:read") && <PipelineBoard jobId={j.id} canUpdate={permissions.includes("pipeline:update")} />}
      <JobSalaryPanel job={j} canEdit={permissions.includes("jobs.salary:update")} />
      <Timeline subjectType="job" subjectId={j.id} permissions={permissions} />
    </main>
  );
}
