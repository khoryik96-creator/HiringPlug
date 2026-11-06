// The timeline of stage moves: who moved which candidate to which stage, and
// when, newest first and grouped by day (the dashboard's, or one candidate's).
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { stageMovesKey } from "../api/keys";
import { STAGE } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Move = Schemas["StageMove"];

/** "Today", "Yesterday" or the date, in the viewer's time zone. */
export function dayLabel(at: Date, now = new Date()): string {
  const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const diff = Math.round((day(now) - day(at)) / 86_400_000);
  if (diff === 0) return "Today";
  if (diff === 1) return "Yesterday";
  return at.toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short", year: at.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

/** Moves grouped by day, keeping their order. */
export function byDay(moves: Move[], now = new Date()): [string, Move[]][] {
  const out: [string, Move[]][] = [];
  for (const m of moves) {
    const label = dayLabel(new Date(m.changedAt), now);
    const last = out[out.length - 1];
    if (last?.[0] === label) last[1].push(m);
    else out.push([label, [m]]);
  }
  return out;
}

export function StageMoves({ candidateId, title = "Recent moves", limit = 30 }: { candidateId?: string; title?: string; limit?: number }) {
  const moves = useQuery({
    queryKey: stageMovesKey(candidateId),
    // The whole list ({ items }), as every query under applicationsKey: a move updates them all in place.
    queryFn: async () => unwrap(await api.GET("/v1/pipeline/moves", { params: { query: { candidateId, limit } } })),
  });
  const items = moves.data?.items;
  return (
    <section className="panel" aria-label={title}>
      <h3>{title}</h3>
      <ErrorMessage error={moves.error} />
      {items?.length === 0 && <p className="muted">No moves yet: candidates appear here when they are added to a job or change stage.</p>}
      {items && byDay(items).map(([day, items]) => (
        <div key={day} className="timeline-day">
          <h4>{day}</h4>
          <ol className="timeline">
            {items.map((m) => (
              <li key={m.id}>
                <time dateTime={m.changedAt}>{new Date(m.changedAt).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}</time>{" "}
                {!candidateId && <><a href={`#/candidates/${m.candidateId}`}>{m.candidateName}</a>{" "}</>}
                {m.fromStage ? (
                  <><span className={`badge ${m.fromStage}`}>{STAGE[m.fromStage]}</span> → </>
                ) : (
                  <span className="muted">added at </span>
                )}
                <span className={`badge ${m.toStage}`}>{STAGE[m.toStage]}</span>{" "}
                <span className="muted">on</span> <a href={`#/jobs/${m.jobId}`}>{m.jobTitle}</a> <span className="muted">({m.companyName})</span>
                {m.changedByName && <span className="muted"> · by {m.changedByName}</span>}
              </li>
            ))}
          </ol>
        </div>
      ))}
    </section>
  );
}
