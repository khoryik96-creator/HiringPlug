import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api, type Schemas, unwrap } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import { Field } from "../Field";
import { fieldErrors, FormInvalid } from "../forms";
import { zonedInput, wallTime, localInput, startDay, addDays } from "./dates";

type Event = Schemas["CalendarEvent"];
export function EventForm({ event, date, connection, onClose, onSaved }: { event?: Event; date: Date; connection: Schemas["OutlookConnection"]; onClose: () => void; onSaved: () => void }) {
  const zone = event?.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
  const start = new Date(event?.startsAt ?? date);
  if (!event) start.setHours(Math.max(9, start.getHours()), 0, 0, 0);
  const [form, setForm] = useState({ title: event?.title ?? "", notes: event?.notes ?? "", location: event?.location ?? "", startsAt: zonedInput(start, zone), endsAt: zonedInput(new Date(event?.endsAt ?? start.getTime() + 3600000), zone), attendees: event?.attendees.map((a) => a.email).join(", ") ?? "", teams: event?.teams ?? false, allDay: event?.allDay ?? false });
  const save = useMutation({ mutationFn: async () => {
    let from: Date, to: Date;
    try { from = wallTime(form.startsAt, zone); } catch (error) { throw new FormInvalid({ startsAt: error instanceof Error ? error.message : "Choose a valid start." }); }
    try { to = wallTime(form.endsAt, zone); } catch (error) { throw new FormInvalid({ endsAt: error instanceof Error ? error.message : "Choose a valid end." }); }
    if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime()) || to <= from) throw new FormInvalid({ endsAt: "Choose an end after the start." });
    const body: Schemas["CalendarEventInput"] = { ...form, startsAt: from.toISOString(), endsAt: to.toISOString(), timeZone: zone, attendees: form.attendees.split(/[,;\n]+/).map((email) => email.trim()).filter(Boolean).map((email) => ({ email })) };
    if (event) return unwrap(await api.PUT("/v1/calendar/events/{eventId}", { params: { path: { eventId: event.id } }, body: { ...body, version: event.version } }));
    return unwrap(await api.POST("/v1/calendar/events", { body }));
  }, onSuccess: onSaved });
  const errors = fieldErrors(save.error);
  function allDay(value: boolean) { setForm((f) => ({ ...f, allDay: value, startsAt: localInput(startDay(new Date(f.startsAt))), endsAt: localInput(addDays(startDay(new Date(f.startsAt)), 1)) })); }
  return <section className="panel calendar-form" role="dialog" aria-modal="true" aria-label={event ? "Edit appointment" : "New appointment"}>
    <div className="calendar-heading"><h3>{event ? "Edit appointment" : "New appointment"}</h3><button type="button" onClick={onClose} disabled={save.isPending}>Close</button></div>
    <form className="form-grid" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <div className="full"><Field label="Title" error={errors.title}><input autoFocus required maxLength={200} value={form.title} onChange={(e) => { setForm({ ...form, title: e.target.value }); }}/></Field></div>
      <Field label="Start" error={errors.startsAt}><input required type="datetime-local" value={form.startsAt} onChange={(e) => { setForm({ ...form, startsAt: e.target.value }); }}/></Field>
      <Field label="End" error={errors.endsAt}><input required type="datetime-local" value={form.endsAt} onChange={(e) => { setForm({ ...form, endsAt: e.target.value }); }}/></Field>
      <div className="row full"><small className="muted">{zone}</small><label><input type="checkbox" checked={form.allDay} onChange={(e) => { allDay(e.target.checked); }}/> All day</label></div>
      <Field label="Location" error={errors.location}><input maxLength={500} value={form.location} onChange={(e) => { setForm({ ...form, location: e.target.value }); }}/></Field>
      <Field label="Attendees" error={errors.attendees ?? Object.entries(errors).find(([key]) => key.startsWith("attendees"))?.[1]}><input disabled={!connection.connected} type="text" value={form.attendees} onChange={(e) => { setForm({ ...form, attendees: e.target.value }); }}/></Field>
      <div className="full"><Field label="Notes" error={errors.notes}><textarea rows={3} maxLength={4000} value={form.notes} onChange={(e) => { setForm({ ...form, notes: e.target.value }); }}/></Field></div>
      {connection.teamsAvailable && <label className="full"><input type="checkbox" checked={form.teams} disabled={event?.teams} onChange={(e) => { setForm({ ...form, teams: e.target.checked }); }}/> Teams meeting</label>}
      <div className="full"><ErrorMessage error={save.error}/></div>
      <div className="row full"><button className="primary" disabled={save.isPending} type="submit">{save.isPending ? "Saving…" : form.attendees.trim() ? event ? "Save & send updates" : "Create & send invitations" : "Save appointment"}</button><button type="button" onClick={onClose} disabled={save.isPending}>Cancel</button></div>
    </form>
  </section>;
}
