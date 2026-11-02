import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Candidate, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

const REASON = { same_phone: "same phone", similar_name: "similar name" } as const;

/**
 * A warning listing possible duplicates of a candidate (nothing if none).
 * With candidates:merge, a duplicate can be merged into this candidate.
 */
export function SimilarCandidates({ candidate, canMerge }: { candidate: Candidate; canMerge: boolean }) {
  const queryClient = useQueryClient();
  const similarKey = ["candidate", candidate.id, "similar"] as const;
  const similar = useQuery({
    queryKey: similarKey,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/similar", { params: { path: { candidateId: candidate.id } } })).items,
  });
  const merge = useMutation({
    mutationFn: async (duplicateId: string) => unwrap(await api.POST("/v1/candidates/{candidateId}/merge", {
      params: { path: { candidateId: candidate.id } }, body: { duplicateId, version: candidate.version },
    })),
    onSuccess: (merged) => {
      queryClient.setQueryData(candidateKey(candidate.id), merged);
      void queryClient.invalidateQueries(); // CVs, jobs, notes and lists all changed
    },
  });
  if (!similar.data || similar.data.length === 0) return null;
  return (
    <section className="panel warning-panel" aria-label="Possible duplicates">
      <strong>Possible duplicate{similar.data.length === 1 ? "" : "s"}:</strong>
      <ul>
        {similar.data.map((s) => (
          <li key={s.candidate.id}>
            <a href={`#/candidates/${s.candidate.id}`}>{s.candidate.fullName}</a>{" "}
            <span className="muted">
              ({s.reasons.map((r) => REASON[r]).join(", ")}{s.candidate.currentTitle ? `; ${s.candidate.currentTitle}` : ""}{s.candidate.email ? `; ${s.candidate.email}` : ""})
            </span>{" "}
            {canMerge && (
              <button
                type="button"
                className="link"
                disabled={merge.isPending}
                onClick={() => {
                  if (window.confirm(`Merge ${s.candidate.fullName} into ${candidate.fullName}? Their CVs, jobs and notes move here, empty fields here are filled from theirs, and ${s.candidate.fullName} is deleted.`)) {
                    merge.mutate(s.candidate.id);
                  }
                }}
              >
                Merge into this one
              </button>
            )}
          </li>
        ))}
      </ul>
      <ErrorMessage error={merge.error} />
    </section>
  );
}
