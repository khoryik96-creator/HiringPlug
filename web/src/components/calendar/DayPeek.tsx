// Beside the composer, as in Outlook: the chosen day in the user's calendar,
// with the meeting being made drawn on it, so a clash shows before sending.
import { useQuery } from "@tanstack/react-query";

import { api, unwrap } from "../../api/client";
import { addDays, startDay, timeLabel } from "./dates";

const HOUR = 3600000;
const ROW = 34; // px per hour

/** skip: the meeting being changed (an appointment's id, or the interview it is), not shown against itself. */
export function DayPeek({ day, start, end, title, skip }: { day: Date; start: Date | null; end: Date | null; title: string; skip?: string }) {
  const from = startDay(day);
  const to = addDays(from, 1);
  const events = useQuery({
    queryKey: ["calendar", from.toISOString(), to.toISOString()],
    queryFn: async () => unwrap(await api.GET("/v1/calendar/events", { params: { query: { from: from.toISOString(), to: to.toISOString() } } })),
    staleTime: 30_000,
  });
  const items = (events.data?.items ?? []).filter((e) => e.status !== "cancelled" && !(skip && (e.id === skip || e.sourceId === skip)));
  const timed = items.filter((e) => !e.allDay);
  const allDay = items.filter((e) => e.allDay);
  const hourOf = (d: Date) => (d.getTime() - from.getTime()) / HOUR;
  const shown = [...timed.map((e) => new Date(e.startsAt)), ...(start ? [start] : [])].map(hourOf).filter((h) => h >= 0 && h < 24);
  const ends = [...timed.map((e) => new Date(e.endsAt)), ...(end ? [end] : [])].map(hourOf).filter((h) => h > 0);
  const first = Math.max(0, Math.min(8, ...shown.map(Math.floor)));
  const last = Math.min(24, Math.max(18, ...ends.map(Math.ceil)));
  const top = (d: Date) => (Math.max(first, Math.min(last, hourOf(d))) - first) * ROW;
  const height = (a: Date, b: Date) => Math.max(ROW / 2.2, top(b) - top(a));
  const clash = start && end ? timed.some((e) => new Date(e.startsAt) < end && new Date(e.endsAt) > start) : false;
  return (
    <section className="day-peek" aria-label="Your day">
      <header>
        <strong>{day.toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long" })}</strong>
        {events.isPending ? <span className="muted small">Loading…</span>
          : events.isError ? <span className="muted small">Your calendar could not be loaded.</span>
            : clash ? <span className="day-peek-clash" role="status">Overlaps your calendar</span>
              : start && <span className="muted small" role="status">You are free then</span>}
      </header>
      {allDay.map((e) => <div key={e.id} className="day-peek-allday">{e.title}</div>)}
      <div className="day-peek-grid" style={{ height: (last - first) * ROW }}>
        {Array.from({ length: last - first }, (_, i) => (
          <div key={i} className="day-peek-hour" style={{ top: i * ROW }}>
            <span>{new Date(2000, 0, 1, first + i).toLocaleTimeString(undefined, { hour: "numeric" })}</span>
          </div>
        ))}
        {timed.map((e) => {
          const a = new Date(e.startsAt), b = new Date(e.endsAt);
          return (
            <div key={e.id} className={`day-peek-event calendar-source-${e.source}${start && end && a < end && b > start ? " beside" : ""}`}
              style={{ top: top(a), height: height(a, b) }} title={`${timeLabel(e.startsAt)} ${e.title}`}>
              {e.title}
            </div>
          );
        })}
        {start && end && (
          <div className={`day-peek-new${clash ? " clash" : ""}`} style={{ top: top(start), height: height(start, end) }}>
            {title.trim() || "New meeting"}
          </div>
        )}
      </div>
    </section>
  );
}
