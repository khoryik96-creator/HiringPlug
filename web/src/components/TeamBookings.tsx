// Settings > Booking calendars (owner): everyone's booking set-up at a
// glance. Each person's calendar address stays private to them: only its
// status shows here.
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { formatWhen, timeAgo } from "../labels";
import { DAYS } from "./BookingHours";
import { ErrorMessage } from "./ErrorMessage";

type Row = Schemas["TeamSchedule"];

/** Weekly hours in a few words: "Mon–Fri 09:00–17:00, Sat 10:00–13:00". */
export function hoursText(hours: Schemas["WeeklyHours"][]): string {
  const byDay = [...hours].sort((a, b) => a.day - b.day || a.start.localeCompare(b.start));
  const parts: { from: number; to: number; time: string }[] = [];
  for (const h of byDay) {
    const time = `${h.start}–${h.end}`;
    const last = parts.at(-1);
    if (last?.time === time && last.to === h.day - 1) last.to = h.day;
    else parts.push({ from: h.day, to: h.day, time });
  }
  if (parts.length === 0) return "No hours";
  return parts.map((p) => `${DAYS[p.from - 1] ?? ""}${p.to > p.from ? `–${DAYS[p.to - 1] ?? ""}` : ""} ${p.time}`).join(", ");
}

function Calendar({ c }: { c: Row["calendar"] }) {
  if (!c) return <span className="muted">Not connected</span>;
  if (c.status === "failed") return <span className="warning">Cannot be read{c.error ? `: ${c.error}` : ""}</span>;
  if (c.status === "pending") return <span className="muted">{c.host} · being read…</span>;
  return <span>✓ {c.host}{c.checkedAt && <span className="muted"> · read {timeAgo(c.checkedAt)}</span>}</span>;
}

export function TeamBookings() {
  const team = useQuery({ queryKey: ["schedule", "team"], queryFn: async () => unwrap(await api.GET("/v1/scheduling/team")).items });
  return (
    <main>
      <h2>Booking calendars</h2>
      <ErrorMessage error={team.error} />
      {team.data && (
        <section className="panel">
          <p className="muted small">Each person sets their own hours and calendar under Your account. Calendar addresses stay private to them.</p>
          <table>
            <thead><tr><th>Person</th><th>Hours for calls</th><th>Calendar</th><th>Booking links</th></tr></thead>
            <tbody>
              {team.data.map((r) => (
                <tr key={r.userId}>
                  <td><strong>{r.name}</strong></td>
                  <td>{r.set ? <>{hoursText(r.hours)} <span className="muted small">({r.timeZone})</span></> : <span className="muted">Not set up</span>}</td>
                  <td><Calendar c={r.calendar} /></td>
                  <td>
                    {r.openLinks === 0 && r.upcoming === 0 ? <span className="muted">None</span> : (
                      <>
                        {r.openLinks > 0 && `${String(r.openLinks)} waiting`}
                        {r.openLinks > 0 && r.upcoming > 0 && " · "}
                        {r.upcoming > 0 && `${String(r.upcoming)} booked`}
                        {r.nextAt && <span className="muted small"> · next {formatWhen(r.nextAt)}</span>}
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </main>
  );
}
