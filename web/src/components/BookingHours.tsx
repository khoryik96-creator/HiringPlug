// Account > Booking hours: when you take calls, and your own calendar's
// private address so your meetings count as busy (Google Calendar or Outlook).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { formatWhen } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Schedule = Schemas["Schedule"];
type Hours = Schemas["WeeklyHours"];

export const scheduleKey = ["schedule", "me"] as const;
export const DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/** The zones the browser knows (all IANA names), or a few common ones. */
function timeZones(current: string): string[] {
  const intl = Intl as unknown as { supportedValuesOf?: (k: string) => string[] };
  const all = intl.supportedValuesOf?.("timeZone") ?? ["Asia/Kuala_Lumpur", "Asia/Singapore", "Asia/Jakarta", "Asia/Bangkok", "Asia/Manila", "Asia/Hong_Kong", "Europe/London", "UTC"];
  return all.includes(current) ? all : [current, ...all];
}

export function BookingHours() {
  const queryClient = useQueryClient();
  const schedule = useQuery({
    queryKey: scheduleKey,
    queryFn: async () => unwrap(await api.GET("/v1/scheduling/me")),
    // Follow a calendar address still being read, for a couple of minutes at most.
    refetchInterval: (q) => (q.state.data?.calendar?.status === "pending" && q.state.dataUpdateCount < 60 ? 2000 : false),
  });
  const s = schedule.data;
  if (!s) return <ErrorMessage error={schedule.error} />;
  // A fresh form only when the saved hours change, never on a background refresh (that would throw away typing).
  const saved = JSON.stringify([s.set, s.timeZone, s.hours, s.bufferMinutes, s.minNoticeHours, s.maxDaysAhead]);
  return <HoursForm key={saved} schedule={s} onSaved={(next) => { queryClient.setQueryData(scheduleKey, next); }} />;
}

function HoursForm({ schedule, onSaved }: { schedule: Schedule; onSaved: (s: Schedule) => void }) {
  const [form, setForm] = useState({
    timeZone: schedule.set ? schedule.timeZone : Intl.DateTimeFormat().resolvedOptions().timeZone,
    hours: schedule.hours, bufferMinutes: schedule.bufferMinutes, minNoticeHours: schedule.minNoticeHours, maxDaysAhead: schedule.maxDaysAhead,
  });
  const [calendarUrl, setCalendarUrl] = useState("");
  const save = useMutation({
    mutationFn: async (calendar?: string | null) => unwrap(await api.PUT("/v1/scheduling/me", {
      body: { ...form, ...(calendar !== undefined ? { calendarUrl: calendar } : {}) },
    })),
    onSuccess: (s) => { setCalendarUrl(""); onSaved(s); },
  });
  const byDay = (d: number) => form.hours.filter((h) => h.day === d);
  const setDay = (d: number, list: Hours[]) => { setForm({ ...form, hours: [...form.hours.filter((h) => h.day !== d), ...list] }); };
  const num = (k: "bufferMinutes" | "minNoticeHours" | "maxDaysAhead") => (e: { target: { value: string } }) => { setForm({ ...form, [k]: Number(e.target.value) }); };
  const cal = schedule.calendar;

  return (
    <section className="panel booking-hours" aria-label="Booking hours">
      <h3>Booking hours</h3>
      {!schedule.set && <p className="warning small">Not saved yet: booking links need your hours.</p>}
      <form onSubmit={(e) => { e.preventDefault(); save.mutate(calendarUrl.trim() ? calendarUrl.trim() : undefined); }}>
        <div className="hours-grid">
          {DAYS.map((label, i) => {
            const d = i + 1;
            const list = byDay(d);
            return (
              <div key={d} className="hours-day">
                <label className="hours-toggle">
                  <input type="checkbox" checked={list.length > 0}
                    onChange={(e) => { setDay(d, e.target.checked ? [{ day: d, start: "09:00", end: "17:00" }] : []); }} />
                  {label}
                </label>
                {list.length === 0 ? <span className="muted small">Unavailable</span> : list.map((h, j) => (
                  <span key={j} className="hours-range">
                    <input type="time" aria-label={`${label} from`} value={h.start} step={900}
                      onChange={(e) => { setDay(d, list.map((x, k) => (k === j ? { ...x, start: e.target.value } : x))); }} />
                    <span className="muted small">to</span>
                    <input type="time" aria-label={`${label} to`} value={h.end} step={900}
                      onChange={(e) => { setDay(d, list.map((x, k) => (k === j ? { ...x, end: e.target.value } : x))); }} />
                  </span>
                ))}
              </div>
            );
          })}
        </div>
        <div className="form-grid">
          <label className="field">Time zone
            <select value={form.timeZone} onChange={(e) => { setForm({ ...form, timeZone: e.target.value }); }}>
              {timeZones(form.timeZone).map((z) => <option key={z} value={z}>{z.replace(/_/g, " ")}</option>)}
            </select>
          </label>
          <label className="field">Gap around meetings (minutes)<input type="number" min={0} max={120} value={form.bufferMinutes} onChange={num("bufferMinutes")} /></label>
          <label className="field">Earliest booking (hours ahead)<input type="number" min={0} max={336} value={form.minNoticeHours} onChange={num("minNoticeHours")} /></label>
          <label className="field">Bookable up to (days ahead)<input type="number" min={1} max={60} value={form.maxDaysAhead} onChange={num("maxDaysAhead")} /></label>
        </div>
        <h4>Your calendar</h4>
        {cal && (
          <p className="row">
            <span className={`badge ${cal.status === "ok" ? "valid" : cal.status === "failed" ? "invalid" : "queued"}`}>
              {cal.status === "ok" ? "Connected" : cal.status === "failed" ? "Not read" : "Reading…"}
            </span>
            <span className="muted small">{cal.host}{cal.checkedAt ? ` · read ${formatWhen(cal.checkedAt)} · ${String(cal.busyCount)} busy times ahead` : ""}</span>
            <button type="button" className="link" disabled={save.isPending} onClick={() => { save.mutate(null); }}>Disconnect</button>
          </p>
        )}
        {cal?.error && <p className="warning small">{cal.error}</p>}
        <label className="field full">{cal ? "New calendar address" : "Calendar address (.ics)"}
          <input type="url" autoComplete="off" spellCheck={false} value={calendarUrl} onChange={(e) => { setCalendarUrl(e.target.value); }} />
        </label>
        <details className="small calendar-help">
          <summary>Where to find it</summary>
          <p><strong>Google Calendar:</strong> Settings › your calendar › Integrate calendar › Secret address in iCal format.</p>
          <p><strong>Outlook:</strong> Settings › Calendar › Shared calendars › Publish a calendar › “Can view when I&apos;m busy” › Publish › ICS link.</p>
          <p>Only busy times are read, never what your meetings are.</p>
        </details>
        <ErrorMessage error={save.error} />
        <button type="submit" className="primary" disabled={save.isPending || form.hours.length === 0}>Save</button>
      </form>
    </section>
  );
}
