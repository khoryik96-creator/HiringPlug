import { useEffect, useRef, useState } from "react";
import type { Schemas } from "../../api/client";
import { addDays, dateLabel, sameDay, startDay, timeLabel } from "./dates";

type Event = Schemas["CalendarEvent"];
const hour = 3600000;
const rowHeight = 64;

type Column = {
  day: Date;
  from: number;
  to: number;
  events: Event[];
};
type Block = { event: Event; start: number; end: number; lane: number; columns: number };

function offset(instant: number, column: Column, week: boolean) {
  if (instant <= column.from) return 0;
  if (instant >= column.to) return week ? 24 : (column.to - column.from) / hour;
  if (!week) return (instant - column.from) / hour;
  const wall = new Date(instant);
  return wall.getHours() + wall.getMinutes() / 60 + wall.getSeconds() / 3600;
}

function placeEvents(column: Column, week: boolean, hours: number): Block[] {
  const blocks = column.events.filter((event) => !event.allDay).map((event) => {
    const from = offset(Date.parse(event.startsAt), column, week);
    const to = Math.min(hours, Math.max(offset(Date.parse(event.endsAt), column, week), from + .5));
    return { event, start: from, end: to, lane: 0, columns: 1 };
  }).sort((a, b) => a.start - b.start || b.end - a.end);

  let group: Block[] = [];
  let laneEnds: number[] = [];
  for (const block of blocks) {
    if (laneEnds.every((due) => due <= block.start)) {
      for (const member of group) member.columns = laneEnds.length;
      group = [];
      laneEnds = [];
    }
    const available = laneEnds.findIndex((due) => due <= block.start);
    block.lane = available < 0 ? laneEnds.length : available;
    laneEnds[block.lane] = block.end;
    group.push(block);
  }
  for (const block of group) block.columns = laneEnds.length;
  return blocks;
}

function atHour(from: number, index: number, week: boolean) {
  if (!week) return new Date(from + index * hour);
  const date = new Date(from);
  date.setHours(index);
  return date;
}

export function CalendarTimeGrid({ days, events, onOpen, onCreate }: {
  days: Date[];
  events: Event[];
  onOpen: (event: Event) => void;
  onCreate?: (start: Date) => void;
}) {
  const week = days.length > 1;
  const [today] = useState(() => startDay(new Date()));
  const first = days[0] ?? today;
  const from = first.getTime();
  const hours = week ? 24 : (addDays(first, 1).getTime() - from) / hour;
  const columns = days.map((day): Column => {
    const from = day.getTime();
    const to = addDays(day, 1).getTime();
    return { day, from, to, events: events.filter((event) => Date.parse(event.startsAt) < to && Date.parse(event.endsAt) > from) };
  });
  const rows = Array.from({ length: Math.ceil(hours) }, (_, index) => index);
  const height = (index: number) => rowHeight * Math.min(1, hours - index);
  const viewport = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const morning = startDay(new Date(from));
    morning.setHours(8);
    if (viewport.current) viewport.current.scrollTop = (week ? 8 : (morning.getTime() - from) / hour) * rowHeight;
  }, [from, week]);
  const allDay = columns.map((column) => column.events.filter((event) => event.allDay));
  const gridTemplateColumns = `var(--day-label-width) repeat(${days.length}, minmax(0,1fr))`;
  // Week columns share a wall-clock axis. Use a fixed UTC reference for its
  // labels so a lost or repeated DST hour cannot shift the other days' rows.
  const weekHour = new Intl.DateTimeFormat(undefined, { timeZone: "UTC", hour: "2-digit", minute: "2-digit" });
  const labelFor = (index: number) => week ? weekHour.format(index * hour) : timeLabel(atHour(from, index, false).toISOString());
  // What the hour rows show: "9 AM", as Outlook writes them (labelFor names the slot buttons).
  const weekHourShort = new Intl.DateTimeFormat(undefined, { timeZone: "UTC", hour: "numeric" });
  const shownHour = (index: number) => week ? weekHourShort.format(index * hour) : atHour(from, index, false).toLocaleTimeString(undefined, { hour: "numeric" });
  return <section aria-label={week ? "Week calendar" : "Day calendar"} className="calendar-day-view">
    <div ref={viewport} className="calendar-day-scroll">
      <div className={week ? "calendar-time-width" : undefined}>
        {week && <div className="calendar-time-head" style={{ gridTemplateColumns }}>
          <div />
          {columns.map((column) => <strong key={column.from} className={sameDay(column.day, today) ? "calendar-time-today" : undefined}>{dateLabel(column.day)}</strong>)}
        </div>}
        {allDay.some((day) => day.length > 0) && <div className="calendar-day-all" style={{ gridTemplateColumns }}>
          <span className="small muted">All day</span>
          {allDay.map((day, index) => <div key={columns[index]?.from}>{day.map((event) => <button key={event.id} type="button" className={eventClass(event)} onClick={() => { onOpen(event); }}>{event.title}</button>)}</div>)}
        </div>}
        <div className="calendar-time-body" style={{ gridTemplateColumns }}>
          <div className="calendar-time-labels">
            {rows.map((index) => <time key={index} style={{ height: height(index) }}>{shownHour(index)}</time>)}
          </div>
          {columns.map((column) => <div key={column.from} className="calendar-time-column" role={week ? "group" : undefined} aria-label={week ? dateLabel(column.day) : undefined}>
            {rows.map((index) => {
              const slot = atHour(column.from, index, week);
              const available = !week || slot.getHours() === index;
              return <div key={index} className="calendar-hour" style={{ height: height(index) }}>
                {onCreate && available && <button type="button" aria-label={`New appointment ${week ? dateLabel(column.day) + " " : ""}at ${labelFor(index)}`}
                  onClick={() => { onCreate(slot); }}/>}
              </div>;
            })}
            <div className="calendar-day-timed">
              {placeEvents(column, week, hours).map(({ event, start, end, lane, columns }) => {
                // As Outlook: the title first, then the time; one line when the meeting is short.
                const short = (end - start) * rowHeight < 44;
                return <button key={event.id} type="button"
                  className={`${eventClass(event)}${short ? " calendar-event-short" : ""}${short && columns > 1 ? " calendar-event-narrow" : ""}`} onClick={() => { onOpen(event); }}
                  style={{ top: start * rowHeight, height: (end - start) * rowHeight, left: `${lane / columns * 100}%`, width: `calc(${100 / columns}% - 4px)` }}
                  title={`${timeLabel(event.startsAt)}–${timeLabel(event.endsAt)} · ${event.title}`}>
                  <span className="calendar-event-title">{event.title}</span>
                  <span className="calendar-event-time">{clockRange(event.startsAt, event.endsAt)}</span>
                </button>;
              })}
            </div>
          </div>)}
        </div>
      </div>
    </div>
  </section>;
}

/** "9:00 – 9:30 AM", or "11:30 AM – 1:00 PM" across noon. */
export function clockRange(from: string, to: string): string {
  const at = (iso: string) => new Date(iso).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  const [a, b] = [at(from), at(to)];
  const period = (s: string) => /\s?([AP]M)$/i.exec(s)?.[1];
  return period(a) && period(a) === period(b) ? `${a.replace(/\s?[AP]M$/i, "")} – ${b}` : `${a} – ${b}`;
}

function eventClass(event: Event) {
  return `calendar-event calendar-source-${event.source}${event.status === "cancelled" ? " calendar-cancelled" : ""}`;
}
