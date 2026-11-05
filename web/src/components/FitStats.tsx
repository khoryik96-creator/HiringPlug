// CV Studio's CV scoring beside the fit score: the recommendation, six
// ratings with a note each, and the AI's strengths and gaps.
import type { Schemas } from "../api/client";

type Match = Schemas["Match"];

export const DIMENSION: Record<Schemas["MatchDimensionName"], string> = {
  skills: "Technical / functional skills",
  experience: "Experience level",
  industry: "Industry / domain background",
  education: "Education & qualifications",
  leadership: "Leadership & communication",
  keywords: "Keyword & requirement match",
};

export const RECOMMENDATION: Record<Schemas["MatchRecommendation"], string> = {
  shortlist: "Shortlist",
  consider: "Consider with reservations",
  pass: "Pass",
};

/** "high", "mid" or "low", for a rating's colour (CV Studio's 70 and 45). */
export function ratingBand(score: number): string {
  if (score >= 70) return "high";
  return score >= 45 ? "mid" : "low";
}

export function FitStats({ match }: { match: Match }) {
  const notes = match.strengths.length + match.gaps.length > 0;
  if (!match.recommendation && match.dimensions.length === 0 && !notes) return null;
  return (
    <div className="fit-stats">
      {match.recommendation && (
        <p>Recommendation: <span className={`badge reco-${match.recommendation}`}>{RECOMMENDATION[match.recommendation]}</span></p>
      )}
      {match.dimensions.length > 0 && (
        <ul className="fit-dimensions" aria-label="Ratings">
          {match.dimensions.map((d) => (
            <li key={d.name}>
              <div className="row spread"><span>{DIMENSION[d.name]}</span><strong>{d.score}/100</strong></div>
              <div className="fit-bar" role="meter" aria-label={DIMENSION[d.name]} aria-valuemin={0} aria-valuemax={100} aria-valuenow={d.score}>
                <div className={`fit-bar-fill ${ratingBand(d.score)}`} style={{ width: `${String(d.score)}%` }} />
              </div>
              {d.note && <div className="muted small">{d.note}</div>}
            </li>
          ))}
        </ul>
      )}
      {notes && (
        <div className="fit-notes">
          {match.strengths.length > 0 && (
            <div><h5>Strengths</h5><ul>{match.strengths.map((s) => <li key={s}>{s}</li>)}</ul></div>
          )}
          {match.gaps.length > 0 && (
            <div><h5>Gaps to ask about</h5><ul>{match.gaps.map((g) => <li key={g}>{g}</li>)}</ul></div>
          )}
        </div>
      )}
      <p className="muted small">Ratings and notes are the AI&apos;s opinion (CV Studio&apos;s CV scoring); the score is worked out from the quoted requirements.</p>
    </div>
  );
}
