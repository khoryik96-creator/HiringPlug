import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { api, ApiError, type Permission, unwrap } from "../api/client";
import { applicationsKey, jobSuggestionsKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

/** A snippet with the job's words, sent marked [[like this]], highlighted. */
export function Highlighted({ text }: { text: string }) {
  const parts: ReactNode[] = [];
  const re = /\[\[(.*?)\]\]/g;
  let last = 0;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    parts.push(text.slice(last, m.index), <mark key={m.index}>{m[1]}</mark>);
    last = m.index + m[0].length;
  }
  parts.push(text.slice(last));
  return <>{parts}</>;
}

/**
 * Possible candidates for a job: people not on it whose CVs share the most
 * words with the job's text (a keyword search, not the AI). Added to the job,
 * they are scored like everyone else.
 */
export function PossibleCandidates({ jobId, permissions }: { jobId: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const allowed = ["matching:read", "candidates:read", "documents:read", "pipeline:read"].every((p) => permissions.includes(p as Permission));
  const list = useQuery({
    queryKey: jobSuggestionsKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/matches/suggestions", { params: { query: { jobId } } })).items,
    enabled: allowed,
  });
  const add = useMutation({
    mutationFn: async (candidateId: string) =>
      unwrap(await api.POST("/v1/jobs/{jobId}/applications", { params: { path: { jobId } }, body: { candidateId } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: applicationsKey });
      await queryClient.invalidateQueries({ queryKey: jobSuggestionsKey(jobId) });
    },
  });
  if (!allowed) return null;
  const noText = list.error instanceof ApiError && list.error.code === "MATCH_NEEDS_TEXT";
  return (
    <section className="panel" aria-label="Possible candidates">
      <h3>Possible candidates</h3>
      {noText ? <p className="muted">Add a description or an ad to see possible candidates.</p> : <ErrorMessage error={list.error ?? add.error} />}
      {list.data?.length === 0 && <p className="muted">No CV shares this job&apos;s words yet.</p>}
      <ul className="suggestions">
        {list.data?.map((s) => (
          <li key={s.candidateId}>
            <a href={`#/candidates/${s.candidateId}`}>{s.candidateName}</a>
            {s.currentTitle && <span className="muted"> · {s.currentTitle}</span>}{" "}
            {permissions.includes("pipeline:update") && (
              <button type="button" className="link" disabled={add.isPending} onClick={() => { add.mutate(s.candidateId); }}>Add to job</button>
            )}
            <div className="small muted"><Highlighted text={s.snippet} /></div>
          </li>
        ))}
      </ul>
    </section>
  );
}
