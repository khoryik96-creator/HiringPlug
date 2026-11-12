import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Candidate, type Schemas, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";

/** Inline changes use the displayed version, just like the full profile form. */
export function useCandidateUpdate(candidate: Candidate) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (change: Pick<Schemas["CandidateUpdate"], "availability" | "workAuthorization">) =>
      unwrap(await api.PATCH("/v1/candidates/{candidateId}", { params: { path: { candidateId: candidate.id } }, body: { version: candidate.version, ...change } })),
    onSuccess: (updated) => {
      client.setQueryData(candidateKey(candidate.id), updated);
      void client.invalidateQueries({ queryKey: candidatesKey });
    },
    onError: (error) => {
      if (error instanceof ApiError && error.code === "CONFLICT_STALE_VERSION") void client.invalidateQueries({ queryKey: candidateKey(candidate.id) });
    },
  });
}
