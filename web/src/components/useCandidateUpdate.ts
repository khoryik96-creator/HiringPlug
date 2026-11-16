import { type QueryClient, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Candidate, type Schemas, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";

type Change = Pick<Schemas["CandidateUpdate"], "availability" | "workAuthorization" | "specialization" | "skills" | "certifications" | "industrySelections" | "industry">;

/**
 * This page's own saves of each candidate: the version a save started from,
 * and the version it made. A second quick change made before the first came
 * back builds on the first; anyone else's change still comes back as a
 * conflict.
 */
const savesByClient = new WeakMap<QueryClient, Map<string, Map<number, number>>>();

/** Inline changes use the displayed version, just like the full profile form (plus this page's own saves since). */
export function useCandidateUpdate(candidate: Candidate) {
  const client = useQueryClient();
  const ownSaves = savesByClient.get(client) ?? new Map<string, Map<number, number>>();
  savesByClient.set(client, ownSaves);
  return useMutation({
    // One at a time per candidate, so each starts from the one before.
    scope: { id: `candidate-update-${candidate.id}` },
    mutationFn: async (change: Change) => {
      const own = ownSaves.get(candidate.id) ?? new Map<number, number>();
      let version = candidate.version;
      for (let next = own.get(version); next !== undefined && next > version; next = own.get(version)) version = next;
      const updated = unwrap(await api.PATCH("/v1/candidates/{candidateId}", { params: { path: { candidateId: candidate.id } }, body: { version, ...change } }));
      own.set(version, updated.version);
      ownSaves.set(candidate.id, own);
      return updated;
    },
    onSuccess: (updated) => {
      client.setQueryData(candidateKey(candidate.id), updated);
      void client.invalidateQueries({ queryKey: candidatesKey });
    },
    onError: (error) => {
      if (error instanceof ApiError && error.code === "CONFLICT_STALE_VERSION") void client.invalidateQueries({ queryKey: candidateKey(candidate.id) });
    },
  });
}
