import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { applicationsKey, candidateApplicationsKey, jobsKey } from "../api/keys";
import { CLOSE_REASON, STAGE } from "../labels";
import { MoveMenu, timeInStage } from "./ApplicationCard";
import { ApplicationPlacement } from "./ApplicationPlacement";
import { ErrorMessage } from "./ErrorMessage";
import { ApplicationInterviews } from "./Interviews";
import { type Picked, SearchPicker } from "./SearchPicker";
import { useMoveApplication } from "./useMoveApplication";

/** The jobs a candidate is on, with their stage; add them to another job. */
export function CandidateApplications({ candidateId, canUpdate, permissions }: { candidateId: string; canUpdate: boolean; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const list = useQuery({
    queryKey: candidateApplicationsKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/applications", { params: { path: { candidateId } } })),
  });
  const move = useMoveApplication();
  const [job, setJob] = useState<Picked | null>(null);
  const add = useMutation({
    mutationFn: async (jobId: string) =>
      unwrap(await api.POST("/v1/jobs/{jobId}/applications", { params: { path: { jobId } }, body: { candidateId } })),
    onSuccess: () => {
      setJob(null);
      void queryClient.invalidateQueries({ queryKey: applicationsKey });
    },
  });

  return (
    <section className="panel" aria-label="Jobs">
      <h3>Jobs</h3>
      <ErrorMessage error={list.error ?? move.error} />
      {list.data && list.data.items.length === 0 && <p className="muted">Not on any job yet.</p>}
      <ul>
        {list.data?.items.map((a) => (
          <li key={a.id}>
            <a href={`#/jobs/${a.jobId}`}>{a.jobTitle}</a> <span className="muted">at {a.companyName}</span>{" "}
            <span className={`badge ${a.stage}`}>{STAGE[a.stage]}</span>{" "}
            <span className="muted">
              {a.reason ? CLOSE_REASON[a.reason] : `${timeInStage(a)} in this stage`}
            </span>{" "}
            {canUpdate && <MoveMenu app={a} busy={move.isPending} onMove={(m) => { move.mutate(m); }} />}
            {permissions.includes("placements:read") && <ApplicationPlacement applicationId={a.id} stage={a.stage} permissions={permissions} />}
            {permissions.includes("interviews:read") && (
              <ApplicationInterviews applicationId={a.id} closed={["rejected", "withdrawn", "placed"].includes(a.stage)} permissions={permissions} />
            )}
          </li>
        ))}
      </ul>
      {canUpdate && (
        <div className="row">
          <SearchPicker
            value={job}
            onChange={(p) => { setJob(p); add.reset(); }}
            queryKey={jobsKey}
            placeholder="Add to a job: type its title"
            noMatch="No open job matches."
            search={async (q) =>
              unwrap(await api.GET("/v1/jobs", { params: { query: { q, status: "open", limit: 8 } } })).items.map((j) => ({ id: j.id, name: j.title, detail: j.companyName }))}
          />
          {job && <button type="button" disabled={add.isPending} onClick={() => { add.mutate(job.id); }}>Add to job</button>}
          <ErrorMessage error={add.error} />
        </div>
      )}
    </section>
  );
}
