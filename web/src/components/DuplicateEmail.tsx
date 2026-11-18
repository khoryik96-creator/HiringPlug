import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, ApiError, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

/**
 * Who already has an email (one candidate per email), with a link to them,
 * inside an error. With merge (candidates:merge), this candidate can be merged
 * into them at once: its CVs, jobs and notes move there, the empty fields there
 * are filled from it, and it is deleted; their page opens.
 */
export function DuplicateEmail({ error, merge }: { error: unknown; merge?: { id: string; name: string } }) {
  const queryClient = useQueryClient();
  const join = useMutation({
    mutationFn: async (keepId: string) => {
      if (!merge) return;
      const keep = unwrap(await api.GET("/v1/candidates/{candidateId}", { params: { path: { candidateId: keepId } } }));
      unwrap(await api.POST("/v1/candidates/{candidateId}/merge", {
        params: { path: { candidateId: keepId } }, body: { duplicateId: merge.id, version: keep.version },
      }));
    },
    onSuccess: (_, keepId) => {
      // Their page at once; this candidate is gone, so its queries are dropped, not asked again (they would 404).
      location.hash = `#/candidates/${keepId}`;
      if (merge) queryClient.removeQueries({ queryKey: candidateKey(merge.id) });
      void queryClient.invalidateQueries(); // CVs, jobs, notes and lists all changed
    },
  });
  if (!(error instanceof ApiError) || error.code !== "CANDIDATE_DUPLICATE_EMAIL") return null;
  const id = typeof error.details.existingCandidateId === "string" ? error.details.existingCandidateId : null;
  const name = typeof error.details.existingCandidateName === "string" ? error.details.existingCandidateName : null;
  const owner = typeof error.details.existingOwnerName === "string" ? error.details.existingOwnerName : null;
  if (!id) return null; // someone you may not see
  const them = name ?? "that candidate";
  return (
    <div className="dup-ask">
      <p>
        <a href={`#/candidates/${id}`}>{name ?? "Open that candidate"}</a>
        {owner ? ` (owned by ${owner})` : ""} already has it. {merge ? "Merge the two, or add the CV to them instead." : "Add the CV to them instead."}
      </p>
      {merge && <div className="row">
        <button type="button" className="doc-btn primary" disabled={join.isPending}
          onClick={() => {
            if (window.confirm(`Merge ${merge.name} into ${them}? The CVs, jobs and notes move to ${them}, empty fields there are filled from ${merge.name}, and ${merge.name} is deleted.`)) {
              join.mutate(id);
            }
          }}>
          {join.isPending ? "Merging…" : `Merge into ${them}`}
        </button>
      </div>}
      <ErrorMessage error={join.error} />
    </div>
  );
}
