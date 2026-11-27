// A contact or company nobody touched for a year (not edited, no note, no
// email, no job): a quiet tag after its name, and the lists' filter.
import { formatWhen } from "../labels";

/** Whether a record last touched then is stale now: over a year ago. */
export function isStale(touchedAt: string, now = new Date()): boolean {
  const yearAgo = new Date(now);
  yearAgo.setFullYear(yearAgo.getFullYear() - 1);
  return new Date(touchedAt).getTime() < yearAgo.getTime();
}

export function StaleTag({ touchedAt }: { touchedAt: string }) {
  if (!isStale(touchedAt)) return null;
  return <span className="stale-tag" title={`Nobody has edited it, written a note, emailed or opened a job for it since ${formatWhen(touchedAt)}`}>Not touched in a year</span>;
}

/** The lists' filter pill: only records nobody touched for a year. */
export function StaleFilter({ on, onChange }: { on: boolean; onChange: (on: boolean) => void }) {
  return <button type="button" className={`search-pill${on ? " on" : ""}`} aria-pressed={on} onClick={() => { onChange(!on); }}>Not touched in a year</button>;
}
