import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationMatchKey, jobMatchesKey, matchesKey } from "../api/keys";
import { formatWhen, MATCH_VERDICT } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { FitStats } from "./FitStats";

export type Match = Schemas["Match"];

const whileScoring = (items: Match[] | undefined) => (items?.some((m) => m.status === "queued") ? 2000 : false);

/** The latest fit score of each application on a job, refreshed while any is waiting for the AI. */
export function useJobMatches(jobId: string, allowed: boolean) {
  return useQuery({
    queryKey: jobMatchesKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/matches", { params: { query: { jobId } } })),
    enabled: allowed,
    refetchInterval: (q) => whileScoring(q.state.data?.items),
  });
}

/** "strong", "partial" or "weak", for the badge's colour. */
export function fitBand(score: number): string {
  if (score >= 75) return "strong";
  return score >= 50 ? "partial" : "weak";
}

/** A short badge: the score, "Scoring…" or "No score". */
export function FitBadge({ match }: { match?: Match }) {
  if (!match) return null;
  if (match.status === "queued") return <span className="badge queued" role="status">Scoring fit…</span>;
  if (match.status === "failed" || match.score === null) return <span className="badge failed">No fit score</span>;
  return (
    <span className={`badge fit-${fitBand(match.score)}`} title={match.outdated ? "The job's description or ad changed since: score it again" : undefined}>
      Fit {match.score}%{match.outdated && " · outdated"}
    </span>
  );
}

/** Why: each requirement quoted from the job, with what the CV shows (folded unless open). */
export function FitReasons({ match, open = false }: { match: Match; open?: boolean }) {
  if (match.status === "queued") return null;
  if (match.status === "failed") return <p className="warning">No fit score: {match.error}</p>;
  return (
    <details className="fit" open={open}>
      <summary>Why {match.score}%?</summary>
      {match.summary && <p>{match.summary}</p>}
      <FitStats match={match} />
      <ul className="fit-reasons">
        {match.reasons.map((r, i) => (
          <li key={i} className={r.verdict}>
            <strong>{MATCH_VERDICT[r.verdict]}:</strong> “{r.requirement}”
            {r.evidence && <div className="small">CV: “{r.evidence}”</div>}
            {r.note && <div className="muted small">{r.note}</div>}
          </li>
        ))}
      </ul>
      {match.outdated && <p className="warning small">The job&apos;s description or ad changed after this score: score it again.</p>}
      {match.scoredAt && <p className="muted small">Scored {formatWhen(match.scoredAt)}{match.model && <> by {match.model}</>}</p>}
    </details>
  );
}

/** "Score fit" for every active candidate on a job (the board's header). */
export function ScoreJobFit({ jobId, enabled, anyScored }: { jobId: string; enabled: boolean; anyScored: boolean }) {
  const queryClient = useQueryClient();
  const score = useMutation({
    mutationFn: async (rescore: boolean) => unwrap(await api.POST("/v1/matches/for-job", { body: { jobId, rescore } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: matchesKey }),
  });
  if (!enabled) return null;
  const r = score.data;
  return (
    <div className="row">
      <button type="button" disabled={score.isPending} onClick={() => { score.mutate(false); }}>Score candidates' fit</button>
      {anyScored && (
        <button type="button" className="link" disabled={score.isPending} onClick={() => { score.mutate(true); }}>Score all again</button>
      )}
      {r && (
        <span className="muted small" role="status">
          {r.queued === 0 ? "Nothing new to score." : `Scoring ${String(r.queued)} candidate(s)…`}
          {r.withoutCv > 0 && ` ${String(r.withoutCv)} without a readable CV left out.`}
          {r.consentWithdrawn > 0 && ` ${String(r.consentWithdrawn)} who withdrew consent left out.`}
        </span>
      )}
      <ErrorMessage error={score.error} />
    </div>
  );
}

/** A candidate's fit for one of their jobs (their page's job list). */
export function ApplicationFit({ applicationId, permissions }: { applicationId: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const allowed = permissions.includes("matching:read");
  const list = useQuery({
    queryKey: applicationMatchKey(applicationId),
    queryFn: async () => unwrap(await api.GET("/v1/matches", { params: { query: { applicationId } } })),
    enabled: allowed,
    refetchInterval: (q) => whileScoring(q.state.data?.items),
  });
  const score = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/matches", { body: { applicationId } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: matchesKey }),
  });
  if (!allowed || !list.data?.enabled) return null;
  const match = list.data.items[0];
  const canScore = permissions.includes("matching:create") && match?.status !== "queued";
  return (
    <div className="application-fit">
      <FitBadge match={match} />{" "}
      {canScore && (
        <button type="button" className="link" disabled={score.isPending} onClick={() => { score.mutate(); }}>
          {match ? "Score again" : "Score fit"}
        </button>
      )}
      <ErrorMessage error={score.error ?? list.error} />
      {match && <FitReasons match={match} />}
    </div>
  );
}
