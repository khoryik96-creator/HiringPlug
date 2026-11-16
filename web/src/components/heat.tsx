// The heat map of jobs and stages, shared by the dashboard and the jobs page:
// a stronger purple for more people, an amber ring where someone has waited
// over 7 days; a number opens the job's board at that stage.

/** "▲ 3 this week", "▼ 5 vs last week": a change, coloured by direction. */
export function Change({ by, text }: { by: number; text: string }) {
  if (by === 0) return <span className="change flat">{text.replace("{n}", "no change")}</span>;
  return <span className={`change ${by > 0 ? "up" : "down"}`}>{by > 0 ? "▲" : "▼"} {text.replace("{n}", String(Math.abs(by)))}</span>;
}

/** How many are in a stage of a job, as a coloured link to the job at that stage. */
export function Heat({ n, waiting, href, label }: { n: number; waiting: boolean; href: string; label: string }) {
  if (n === 0) return <span className="heat h0" aria-label="none">·</span>;
  const level = n >= 4 ? "h3" : n >= 2 ? "h2" : "h1";
  return (
    <a href={href} className={`heat ${level}${waiting ? " stall" : ""}`} aria-label={label}
      title={waiting ? "Someone here has waited over 7 days · open the job" : "Open the job"}>{n}</a>
  );
}

/** What the colours mean. */
export function HeatLegend({ extra }: { extra?: string }) {
  return (
    <p className="heat-legend muted small">
      <span><i className="heat-key h1" />1</span><span><i className="heat-key h2" />2–3</span><span><i className="heat-key h3" />4+</span>
      <span><i className="heat-key ring" />someone waiting over 7 days</span>
      {extra && <span>{extra}</span>}
    </p>
  );
}

/** When a job last saw a candidate join or move, in words; old after a week. */
export function lastActivity(iso: string | null | undefined, now: number): { text: string; old: boolean } {
  if (!iso) return { text: "no candidates", old: true };
  const days = Math.floor((now - Date.parse(iso)) / 86_400_000);
  return { text: days < 1 ? "today" : days === 1 ? "yesterday" : `${String(days)} days ago`, old: days >= 7 };
}
