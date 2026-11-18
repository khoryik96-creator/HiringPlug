import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationMatchKey, jobMatchesKey, matchesKey } from "../api/keys";
import { formatWhen, MATCH_VERDICT } from "../labels";
import { SparkIcon } from "./ActionIcons";
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
/** compact: on a board line, "outdated" is a ↻ (said in full to screen readers and in the tooltip). */
export function FitBadge({ match, compact = false }: { match?: Match; compact?: boolean }) {
  if (!match) return null;
  if (match.status === "queued") return <span className="badge queued" role="status">Scoring fit…</span>;
  if (match.status === "failed" || match.score === null) return <span className="badge failed">No fit score</span>;
  return (
    <span className={`badge fit-${fitBand(match.score)}`} title={match.outdated ? "The job's description or ad changed since: score it again" : undefined}>
      Fit {match.score}%{match.outdated && (compact ? <><span aria-hidden="true"> ↻</span><span className="visually-hidden"> · outdated</span></> : " · outdated")}
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

/**
 * Score on a job's pipeline: with people chosen, scores them (again); else
 * everyone not scored yet, or scored from an older job text.
 */
export function ScoreJobFit({ jobId, enabled, chosen = [] }: { jobId: string; enabled: boolean; chosen?: string[] }) {
  const queryClient = useQueryClient();
  const score = useMutation({
    mutationFn: async (ids: string[]): Promise<{ queued: number; withoutCv?: number; consentWithdrawn?: number; failed?: string[] }> => {
      if (ids.length === 0) return unwrap(await api.POST("/v1/matches/for-job", { body: { jobId, rescore: false } }));
      // A few at a time, not one request per person at once.
      const failed: string[] = [];
      for (let i = 0; i < ids.length; i += 4) {
        const done = await Promise.allSettled(ids.slice(i, i + 4).map(async (applicationId) => unwrap(await api.POST("/v1/matches", { body: { applicationId } }))));
        failed.push(...done.flatMap((d) => (d.status === "rejected" ? [d.reason instanceof Error ? d.reason.message : String(d.reason)] : [])));
      }
      return { queued: ids.length - failed.length, failed };
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: matchesKey }),
  });
  if (!enabled) return null;
  const r = score.data;
  const n = chosen.length;
  return (
    <>
      {r && (
        <span className="muted small" role="status">
          {r.queued > 0 ? `Scoring ${String(r.queued)}…` : r.failed ? "" : "Everyone is scored. Choose people to score them again."}
          {(r.withoutCv ?? 0) > 0 && ` ${String(r.withoutCv)} without a readable CV left out.`}
          {(r.consentWithdrawn ?? 0) > 0 && ` ${String(r.consentWithdrawn)} who withdrew consent left out.`}
          {r.failed && r.failed.length > 0 && ` ${String(r.failed.length)} not scored: ${r.failed[0] ?? ""}`}
        </span>
      )}
      <button type="button" className="doc-btn score-btn" disabled={score.isPending}
        title={n > 0 ? `Score the ${String(n)} chosen again` : "Score everyone not scored yet (choose people to score them again)"}
        onClick={() => { score.mutate(chosen); }}><SparkIcon />{n > 0 ? `Score ${String(n)}` : "Score"}</button>
      <ErrorMessage error={score.error} />
    </>
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
