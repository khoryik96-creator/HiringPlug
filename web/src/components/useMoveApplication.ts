import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, type Application, type Schemas, unwrap } from "../api/client";
import { applicationsKey } from "../api/keys";

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
export function useMoveApplication() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ app, stage, reason, reasonNote }: MoveRequest) => {
      const body: Schemas["ApplicationMove"] = { version: app.version, stage };
      if (reason) body.reason = reason;
      if (reasonNote?.trim()) body.reasonNote = reasonNote.trim();
      return unwrap(await api.POST("/v1/applications/{applicationId}/move", {
        params: { path: { applicationId: app.id } }, body,
      }));
    },
    onSuccess: (moved) => {
      // Update every cached list showing this application, then refetch them.
      queryClient.setQueriesData<{ items: Application[] }>({ queryKey: applicationsKey }, (list) =>
        list ? { items: list.items.map((a) => (a.id === moved.id ? moved : a)) } : list);
      void queryClient.invalidateQueries({ queryKey: applicationsKey });
    },
  });
}
