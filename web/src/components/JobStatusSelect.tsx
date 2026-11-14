import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, type JobOrder, type Schemas, unwrap } from "../api/client";
import { jobKey, jobsKey } from "../api/keys";
import { JOB_STATUS } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

/** A job's status: a coloured badge, or a dropdown of the same colour for those who may change it. */
export function JobStatusSelect({ job, canChange }: { job: JobOrder; canChange: boolean }) {
  const queryClient = useQueryClient();
  const save = useMutation({
    mutationFn: async (status: Schemas["JobStatus"]) =>
      unwrap(await api.PATCH("/v1/jobs/{jobId}", { params: { path: { jobId: job.id } }, body: { version: job.version, status } })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(jobKey(job.id), saved);
      await queryClient.invalidateQueries({ queryKey: jobsKey });
    },
  });
  if (!canChange) return <span className={`badge ${job.status}`}>{JOB_STATUS[job.status]}</span>;
  const shown = save.isPending ? save.variables : job.status; // the choice, while it saves
  return (
    <span className="job-status">
      <select aria-label="Job status" className={`badge status-select ${shown}`} value={shown} disabled={save.isPending}
        onChange={(e) => { save.mutate(e.target.value as Schemas["JobStatus"]); }}>
        {(Object.keys(JOB_STATUS) as Schemas["JobStatus"][]).map((s) => <option key={s} value={s}>{JOB_STATUS[s]}</option>)}
      </select>
      {save.error && <ErrorMessage error={save.error} />}
    </span>
  );
}
