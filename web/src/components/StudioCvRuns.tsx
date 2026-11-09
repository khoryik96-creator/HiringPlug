// CV Studio formatting runs for a candidate: one being made, one saved (with
// CV Studio's warning to compare it with the CV), or why one failed.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

import { api, unwrap } from "../api/client";
import { studioCvsKey } from "./FormatCv";

const DAY = 24 * 3600 * 1000;

/** documentIds: the candidate's documents now (a run whose CV was deleted is left out). */
export function StudioCvRuns({ candidateId, documentIds }: { candidateId: string; documentIds?: Set<string> }) {
  const queryClient = useQueryClient();
  const runs = useQuery({
    queryKey: studioCvsKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/studio-cvs", { params: { path: { candidateId } } })).items,
    refetchInterval: (q) => (q.state.data?.some((r) => r.status === "queued") ? 3000 : false),
  });
  // A run just finished: its CV joins the documents.
  const queued = useRef(new Set<string>());
  useEffect(() => {
    const now = new Set(runs.data?.filter((r) => r.status === "queued").map((r) => r.id));
    if ([...queued.current].some((id) => !now.has(id))) void queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
    queued.current = now;
  }, [runs.data, candidateId, queryClient]);
  const fetchedAt = runs.dataUpdatedAt;
  const recent = (runs.data ?? [])
    .filter((r) => r.status === "queued" || fetchedAt - new Date(r.createdAt).getTime() < DAY)
    .filter((r) => !(r.status === "done" && r.documentId && documentIds && !documentIds.has(r.documentId)))
    .slice(0, 3);
  if (recent.length === 0) return null;
  return (
    <ul className="studio-runs" aria-label="CV Studio formatting">
      {recent.map((r) => (
        <li key={r.id} className={`studio-run ${r.status}`}>
          {r.status === "queued" && <span role="status">Formatting the CV CV Studio&apos;s way{r.blind ? " (blind)" : ""}… this takes a minute.</span>}
          {r.status === "done" && (
            <>
              <span>Formatted CV saved: <strong>{r.fileName}</strong>{r.payStatementsRemoved > 0 && <span className="muted"> (salary left out)</span>}</span>
              {r.warning && <p className="warning">Compare it with the CV before sending: {r.warning}</p>}
            </>
          )}
          {r.status === "failed" && <p className="warning" role="alert">{r.error}</p>}
        </li>
      ))}
    </ul>
  );
}
