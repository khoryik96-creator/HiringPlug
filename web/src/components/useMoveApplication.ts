import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, type Application, type Schemas, unwrap } from "../api/client";
import { applicationsKey, placementsKey } from "../api/keys";

export interface MoveRequest {
  app: Application;
  stage: Schemas["Stage"];
  reason?: Schemas["CloseReason"];
  reasonNote?: string;
}

/**
 * Moves an application to another stage. It sends the version the card
 * showed, so if someone else moved it meanwhile the API answers
 * CONFLICT_STALE_VERSION instead of silently overwriting their move.
 */
async function moveOne({ app, stage, reason, reasonNote }: MoveRequest) {
  const body: Schemas["ApplicationMove"] = { version: app.version, stage };
  if (reason) body.reason = reason;
  if (reasonNote?.trim()) body.reasonNote = reasonNote.trim();
  return unwrap(await api.POST("/v1/applications/{applicationId}/move", {
    params: { path: { applicationId: app.id } }, body,
  }));
}

export function useMoveApplication() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: moveOne,
    onSuccess: (moved) => {
      // Update every cached list showing this application, then refetch them.
      queryClient.setQueriesData<{ items: Application[] }>({ queryKey: applicationsKey }, (list) =>
        list ? { items: list.items.map((a) => (a.id === moved.id ? moved : a)) } : list);
      void queryClient.invalidateQueries({ queryKey: applicationsKey });
      void queryClient.invalidateQueries({ queryKey: placementsKey }); // out of placed: its placement is backed out
    },
  });
}

/**
 * Moves several applications at once (all sent together), then refreshes the
 * lists once. Each keeps its own version check; onMoved gets the ids that
 * moved, and the first failure (if any) is the mutation's error.
 */
export function useMoveApplications(onMoved: (ids: string[]) => void) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (requests: MoveRequest[]) => {
      const results = await Promise.allSettled(requests.map(moveOne));
      onMoved(requests.filter((_, i) => results[i]?.status === "fulfilled").map((r) => r.app.id));
      const failed = results.find((r): r is PromiseRejectedResult => r.status === "rejected");
      if (failed) throw failed.reason;
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: applicationsKey });
      void queryClient.invalidateQueries({ queryKey: placementsKey });
    },
  });
}
