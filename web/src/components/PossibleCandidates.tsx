import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { applicationsKey, jobSuggestionsKey } from "../api/keys";
import { IconButton, PlusIcon } from "./ActionIcons";
import { ErrorMessage } from "./ErrorMessage";
import { readingInterval, WhyLine } from "./search/MeaningSearch";

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
 * Possible candidates for a job: people not on it whose work matches the job
 * by meaning (its title and what its description asks for, read as a
 * recruiter reads them, whatever their titles and CVs say), and those whose
 * CVs share the most words with the job's text. Added to the job, they are
 * scored like everyone else. Folded under the pipeline.
 */
export function PossibleCandidates({ jobId, permissions }: { jobId: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const allowed = ["matching:read", "candidates:read", "documents:read", "pipeline:read"].every((p) => permissions.includes(p as Permission));
  const list = useQuery({
    queryKey: jobSuggestionsKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/matches/suggestions", { params: { query: { jobId } } })),
    enabled: allowed,
    // While the job is being read by meaning (a moment), ask again.
    refetchInterval: (query) => readingInterval(query.state.data?.meaning?.status === "reading", query.state.dataUpdateCount),
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
  const noText = list.data?.needsText === true;
  const found = list.data?.items;
  const meaning = list.data?.meaning;
  const showLevel = Boolean(meaning?.understood?.levelName);
  return (
    <details className="fold" aria-label="Possible candidates">
      <summary>
        <span className="fold-title">Possible candidates</span>
        {found && !noText && <span className="count-chip">{String(found.length)}</span>}
      </summary>
      {noText && <p className="muted">Add a description or an ad to find matches.</p>}
      {meaning?.status === "reading" && (
        <p className="meaning-banner" role="status">
          <span className="spinner" aria-hidden="true" /> Reading the job the way a recruiter would… meanwhile, CVs with its words.
        </p>
      )}
      {meaning?.status === "unavailable" && <p className="muted small">Matching by meaning is not set up on this server: CVs with the job’s words only.</p>}
      <ErrorMessage error={list.error ?? add.error} />
      {found?.length === 0 && !noText && meaning?.status !== "reading" && <p className="muted">No one matches yet.</p>}
      <ul className="suggestions scroll-box">
        {found?.map((s) => {
          const words = matchedWords(s.snippet);
          return (
            <li key={s.candidateId} className="suggestion">
              <div className="suggestion-head">
                <div>
                  <a href={`#/candidates/${s.candidateId}`} className="suggestion-name">{s.candidateName}</a>
                  {s.currentTitle && <div className="muted small">{s.currentTitle}</div>}
                </div>
                {permissions.includes("pipeline:update") && (
                  <IconButton label="Add to job" icon={<PlusIcon />} className="icon-outline" disabled={add.isPending} onClick={() => { add.mutate(s.candidateId); }} />
                )}
              </div>
              {s.why && <WhyLine why={s.why} showLevel={showLevel} showWords={false} />}
              {words.length > 0 && (
                <div className="suggestion-words" aria-label="Matched words">
                  <span className="muted small">{words.length} matched:</span>
                  {words.slice(0, 8).map((w) => <span key={w} className="word-chip">{w}</span>)}
                  {words.length > 8 && <span className="muted small">+{words.length - 8} more</span>}
                </div>
              )}
              {s.snippet && !s.why?.passage && <p className="suggestion-snippet"><Highlighted text={tidySnippet(s.snippet)} /></p>}
            </li>
          );
        })}
      </ul>
    </details>
  );
}
