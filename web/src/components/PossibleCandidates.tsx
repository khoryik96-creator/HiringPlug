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

/** The job's words a snippet shares, once each, in the order they come. */
export function matchedWords(snippet: string): string[] {
  const seen = new Map<string, string>();
  for (const m of snippet.matchAll(/\[\[(.*?)\]\]/g)) {
    const word = m[1]?.trim() ?? "";
    if (word.length > 1 && !seen.has(word.toLowerCase())) seen.set(word.toLowerCase(), word);
  }
  return [...seen.values()];
}

/** Digits that read as a phone number: 9 or more, not just years ("2015 - 2018 2018 - 2020"). */
const isPhone = (m: string) =>
  m.replace(/\D/g, "").length >= 9 && !(m.match(/\d+/g) ?? []).every((g) => /^(19|20)\d\d$/.test(g));

/** A snippet without contact details (they are on the profile) and with tidy gaps. */
export function tidySnippet(snippet: string): string {
  return snippet
    .replace(/[\w.+-]+@[\w-]+(\.[\w-]+)+/g, " ")
    .replace(/\(?\+?\d[\d\s()-]{7,}\d/g, (m) => (isPhone(m) ? " " : m))
    .replace(/[•▪◦●○]/g, "·")
    .replace(/\s{2,}/g, " ")
    .trim();
}

/**
 * Possible candidates for a job: people not on it whose CVs share the most
 * words with the job's text (a keyword search, not the AI). Added to the job,
 * they are scored like everyone else. Folded under the pipeline.
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
    <details className="fold" aria-label="Possible candidates">
      <summary>
        <span className="fold-title">Possible candidates</span>
        {list.data && <span className="count-chip">{String(list.data.length)}</span>}
        <span className="muted small">CVs that share this job&apos;s words</span>
      </summary>
      {noText ? <p className="muted">Add a description or an ad to see possible candidates.</p> : <ErrorMessage error={list.error ?? add.error} />}
      {list.data?.length === 0 && <p className="muted">No CV shares this job&apos;s words yet.</p>}
      <ul className="suggestions scroll-box">
        {list.data?.map((s) => {
          const words = matchedWords(s.snippet);
          return (
            <li key={s.candidateId} className="suggestion">
              <div className="suggestion-head">
                <div>
                  <a href={`#/candidates/${s.candidateId}`} className="suggestion-name">{s.candidateName}</a>
                  {s.currentTitle && <div className="muted small">{s.currentTitle}</div>}
                </div>
                {permissions.includes("pipeline:update") && (
                  <button type="button" className="small-button" disabled={add.isPending} onClick={() => { add.mutate(s.candidateId); }}>+ Add to job</button>
                )}
              </div>
              {words.length > 0 && (
                <div className="suggestion-words" aria-label="Matched words">
                  <span className="muted small">{words.length} matched:</span>
                  {words.slice(0, 8).map((w) => <span key={w} className="word-chip">{w}</span>)}
                  {words.length > 8 && <span className="muted small">+{words.length - 8} more</span>}
                </div>
              )}
              <p className="suggestion-snippet"><Highlighted text={tidySnippet(s.snippet)} /></p>
            </li>
          );
        })}
      </ul>
    </details>
  );
}
