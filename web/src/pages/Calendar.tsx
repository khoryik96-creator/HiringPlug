import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { EventForm } from "../components/calendar/EventForm";
import { EventDetails } from "../components/calendar/EventDetails";
import { CalendarTimeGrid } from "../components/calendar/CalendarTimeGrid";
import { addDays, daysFor, dateLabel, sameDay, timeLabel, type CalendarView } from "../components/calendar/dates";
import "./calendar-workspace.css";

type Event = Schemas["CalendarEvent"];
export function Calendar() {
  const client = useQueryClient();
  const [date, setDate] = useState(new Date()), [mode, setMode] = useState<CalendarView>("month");
  const [slot, setSlot] = useState<Date>();
  const [selected, setSelected] = useState<Event | null>(null), [form, setForm] = useState(false), [editing, setEditing] = useState<Event | undefined>();
  const days = daysFor(date, mode), first = days[0] ?? date, end = addDays(days[days.length - 1] ?? date, 1);
  const query = useQuery({ queryKey: ["calendar", first.toISOString(), end.toISOString()], queryFn: async () => unwrap(await api.GET("/v1/calendar/events", { params: { query: { from: first.toISOString(), to: end.toISOString() } } })), refetchInterval: 15000 });
  const status = query.data?.connection;
  const changed = () => { setForm(false); setSelected(null); setEditing(undefined); void client.invalidateQueries({ queryKey: ["calendar"] }); };
  const connect = useMutation({ mutationFn: async () => unwrap(await api.POST("/v1/calendar/authorize")), onSuccess: (v) => { window.location.assign(v.url); } });
  const refresh = useMutation({ mutationFn: async () => unwrap(await api.POST("/v1/calendar/refresh")), onSuccess: () => { void client.invalidateQueries({ queryKey: ["calendar"] }); } });
  const disconnect = useMutation({ mutationFn: async () => { unwrap(await api.DELETE("/v1/calendar/connection")); }, onSuccess: () => { changed(); } });
  const finish = useMutation({ mutationFn: async (ticket: string) => unwrap(await api.POST("/v1/calendar/complete", { body: { ticket } })), onSuccess: () => { changed(); } });
  const started = useRef(false), [callbackFailed] = useState(() => new URLSearchParams(window.location.hash.split("?")[1]).get("outlook") === "failed");
  useEffect(() => {
    const params = new URLSearchParams(window.location.hash.split("?")[1]), ticket = params.get("ticket");
    if (started.current || params.get("outlook") !== "confirm" || !ticket) return;
    started.current = true; window.history.replaceState(null, "", "#/calendar"); finish.mutate(ticket);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  function move(direction: number) { setDate((d) => mode === "month" ? new Date(d.getFullYear(), d.getMonth() + direction, 1) : addDays(d, direction * (mode === "day" ? 1 : 7))); }
  function newAppointment(start?: Date) {
    setSlot(start);
    setEditing(undefined);
    setForm(true);
  }
  const events = query.data?.items ?? [];
  const onDay = (day: Date) => events.filter((e) => new Date(e.startsAt) < addDays(day, 1) && new Date(e.endsAt) > day);
  const synced = status?.lastSyncAt ? new Date(status.lastSyncAt) : null;
  const syncedDay = synced ? new Date(Date.UTC(synced.getUTCFullYear(), synced.getUTCMonth(), synced.getUTCDate())) : null;
  const outsideOutlookWindow = status?.connected && syncedDay && (first < addDays(syncedDay, -31) || end > addDays(syncedDay, 94));
  const title = mode === "month" ? date.toLocaleDateString(undefined, { month: "long", year: "numeric" })
    : mode === "day" ? date.toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long", year: "numeric" })
      : `${dateLabel(first)} – ${dateLabel(addDays(end, -1))}`;
  return <main className="calendar-page"><div className="calendar-heading"><h2>Calendar</h2><button className="primary" disabled={!status} onClick={() => { newAppointment(); }}>New appointment</button></div>
    <ErrorMessage error={query.error ?? connect.error ?? disconnect.error ?? refresh.error ?? finish.error}/>
    {callbackFailed && <p className="error" role="alert">Outlook sign-in was not completed. Connect again.</p>}
    {finish.isPending && <p role="status">Connecting Outlook…</p>}
    {status && <section className="panel calendar-connection" aria-label="Outlook connection">
      <div><strong>{status.connected ? status.address : "Outlook Calendar"}</strong><span className="muted small">{status.connected ? ` · ${status.status === "ok" ? "Connected" : status.status === "pending" ? "Syncing…" : status.status === "reconnect" ? "Reconnect needed" : "Sync needs attention"}` : " · Your private calendar"}</span></div>
      <div className="row">{status.connected && <><button disabled={refresh.isPending} onClick={() => { refresh.mutate(); }}>Refresh</button><button disabled={disconnect.isPending} onClick={() => { if (window.confirm("Disconnect Outlook? Existing Outlook events stay in Outlook.")) disconnect.mutate(); }}>Disconnect</button></>}<button disabled={!status.ready || connect.isPending} onClick={() => { connect.mutate(); }}>{status.connected ? "Reconnect" : "Connect Outlook"}</button></div>
      {!status.ready && <p className="muted small calendar-wide">An admin needs to configure the Microsoft app in <a href="#/email-connection">Email connection</a>. <a href="https://learn.microsoft.com/en-us/entra/identity-platform/quickstart-register-app" target="_blank" rel="noreferrer">Microsoft setup</a></p>}
      {status.error && <p className="warning calendar-wide" role="alert">{status.error}</p>}
      <details className="small calendar-wide"><summary>Connection details</summary><p>Web redirect URI: <code>{status.callbackUrl}</code></p><p>Microsoft Graph delegated permission: Calendars.ReadWrite. Your admin may need to approve it.</p>{status.lastSyncAt && <p>Last synced: {new Date(status.lastSyncAt).toLocaleString()}</p>}</details>
    </section>}
    <section className="panel calendar-workspace"><div className="calendar-heading calendar-toolbar"><div className="row"><button aria-label="Previous period" onClick={() => { move(-1); }}>‹</button><button onClick={() => { setDate(new Date()); }}>Today</button><button aria-label="Next period" onClick={() => { move(1); }}>›</button><h3>{title}</h3></div><div className="row" aria-label="Calendar view">{(["month", "week", "day", "agenda"] as const).map((view) => <button key={view} aria-pressed={mode === view} onClick={() => { setMode(view); }}>{view.charAt(0).toUpperCase() + view.slice(1)}</button>)}</div></div>
      <p className="muted small">{Intl.DateTimeFormat().resolvedOptions().timeZone}{status?.connected ? " · Outlook shows the past 31 days and next 94 days. Refreshes every 5 minutes." : ""}</p>
      {outsideOutlookWindow && <p role="status" className="warning small">This period extends beyond the synced Outlook window. Appointments outside it may be missing.</p>}
      {query.isPending ? <p role="status">Loading calendar…</p> : mode === "day" || mode === "week"
        ? <CalendarTimeGrid days={days} events={events} onOpen={setSelected} onCreate={status ? newAppointment : undefined}/>
        : mode === "agenda" ? <div className="calendar-agenda">{days.map((day) => <div key={day.toISOString()} className="calendar-agenda-day"><strong>{dateLabel(day)}</strong><div>{onDay(day).length ? onDay(day).map((event) => <EventButton key={event.id} event={event} onClick={() => { setSelected(event); }}/>) : <span className="muted small">No appointments</span>}</div></div>)}</div> : <>
        <div className="calendar-weekdays">{days.slice(0, 7).map((day) => <span key={day.toISOString()}>{day.toLocaleDateString(undefined, { weekday: "short" })}</span>)}</div>
        <div className="calendar-grid">{days.map((day) => <div key={day.toISOString()} className={`calendar-day ${day.getMonth() !== date.getMonth() ? "calendar-outside" : ""}`}><button className={`calendar-date ${sameDay(day, new Date()) ? "calendar-today" : ""}`} aria-label={`New appointment ${dateLabel(day)}`} onClick={() => { setDate(day); newAppointment(); }}>{day.getDate()}</button>{onDay(day).map((event) => <EventButton key={event.id} event={event} onClick={() => { setSelected(event); }}/>)}</div>)}</div>
      </>}
    </section>
    {(form || selected) && <div className="calendar-overlay" onKeyDown={(e) => { if (e.key === "Escape") { setForm(false); setSelected(null); } }}>{form && status ? <EventForm key={editing?.id ?? "new"} event={editing} date={slot ?? date} startAtSlot={slot !== undefined} connection={status} onClose={() => { setForm(false); }} onSaved={changed}/> : selected && <EventDetails event={selected} onClose={() => { setSelected(null); }} onEdit={() => { setEditing(selected); setForm(true); }} onChanged={changed}/>}</div>}
  </main>;
}
function EventButton({ event, onClick }: { event: Event; onClick: () => void }) { return <button className={`calendar-event calendar-source-${event.source} ${event.status === "cancelled" ? "calendar-cancelled" : ""}`} onClick={onClick}><span className="calendar-event-time">{event.allDay ? "All day" : timeLabel(event.startsAt)}</span><span>{event.title}</span>{(event.syncStatus === "failed" || event.syncStatus === "conflict") && <span aria-label="Sync needs attention">!</span>}</button>; }
