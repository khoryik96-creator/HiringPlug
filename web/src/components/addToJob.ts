// Adding a candidate to an open job: the job search a picker uses, and the add
// itself (shared by the candidate's page and the search's resume reader).
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";
import { applicationsKey } from "../api/keys";

/** Open jobs whose title matches, for a SearchPicker. */
export async function searchOpenJobs(q: string) {
  const jobs = unwrap(await api.GET("/v1/jobs", { params: { query: { q, status: "open", limit: 8 } } }));
  return jobs.items.map((j) => ({ id: j.id, name: j.title, detail: j.companyName }));
}

/** Adds the candidate to a job (by id); the boards and the candidate's jobs refresh. */
export function useAddToJob(candidateId: string, onAdded?: () => void) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (jobId: string) =>
      unwrap(await api.POST("/v1/jobs/{jobId}/applications", { params: { path: { jobId } }, body: { candidateId } })),
    onSuccess: () => {
      onAdded?.();
      void queryClient.invalidateQueries({ queryKey: applicationsKey });
    },
  });
}
