// A new or changed appointment, composed as in Outlook: Send (or Save when no
// one is invited), the title, attendees, the time, an online meeting (Teams,
// Google Meet or Zoom), the place and notes, with the day beside it.
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { IconButton, TrashIcon } from "../ActionIcons";
import { plainToHtml } from "../clientEmail/richText";
import { ErrorMessage } from "../ErrorMessage";
import { fieldErrors, FormInvalid } from "../forms";
import { Composer, EmailChips, EndTime, MeetingPicker, NotesIcon, PeopleIcon, PinIcon, Row, SendIcon, StartTime, TimeIcon, VideoIcon } from "./Composer";
import { nextDay, wallTime, zonedInput } from "./dates";
import { bodyText, ComposeBody, hasSignature, saysSomething, SignatureIcon, useMessage } from "./ComposeBody";
import { DayPeek } from "./DayPeek";
import { joinLocation, linkProblem, type Service, splitLocation } from "./meeting";

type Event = Schemas["CalendarEvent"];
const DAY = 24 * 60;
/** Ends Outlook offers: 15, 30 and 45 minutes, then every half hour to a day. */
const ENDS = [15, 30, 45, ...Array.from({ length: 47 }, (_, i) => 60 + i * 30)];

export function EventForm({ event, date, startAtSlot = false, connection, onClose, onSaved }: { event?: Event; date: Date; startAtSlot?: boolean; connection: Schemas["OutlookConnection"]; onClose: () => void; onSaved: () => void }) {
  const zone = event?.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
  const start = new Date(event?.startsAt ?? date);
  if (!event && !startAtSlot) start.setHours(Math.max(9, start.getHours()), 0, 0, 0);
  const end = new Date(event?.endsAt ?? start.getTime() + 3600000);
  const startText = zonedInput(start, zone);
  const minutes = Math.round((end.getTime() - start.getTime()) / 60000);
  const where = splitLocation(event?.location);
  const online: string = where.online; // Teams, Google Meet, Zoom or none, as the picker shows it
  const [form, setForm] = useState({
    title: event?.title ?? "", place: where.place, online, link: where.link,
    attendees: event?.attendees.filter((a) => !a.optional).map((a) => a.email) ?? [],
    optional: event?.attendees.filter((a) => a.optional).map((a) => a.email) ?? [], allDay: event?.allDay ?? false,
    day: startText.slice(0, 10), time: startText.slice(11, 16), minutes: minutes > 0 ? minutes : 60,
    // All day: the last day, as people say it (the stored end is the next midnight).
    lastDay: event?.allDay ? addDayText(zonedInput(end, zone).slice(0, 10), -1) : startText.slice(0, 10),
    // A timed appointment over more than a day keeps its own end.
    longEnd: minutes > DAY && !event?.allDay ? zonedInput(end, zone) : null as string | null,
  });
  const set = (change: Partial<typeof form>) => { setForm((f) => ({ ...f, ...change })); };
  // The notes, formatted; a new appointment starts with the person's signature, as in Outlook.
  const [showOptional, setShowOptional] = useState(form.optional.length > 0);
  const { message, signature, toggleSignature, write } = useMessage(event ? event.notesHtml ?? plainToHtml(event.notes) : "", !event, connection.connected);
  const teamsAuto = connection.teamsAvailable && !event?.teams;
  const service = (["teams", "google_meet", "zoom"] as string[]).includes(form.online) ? form.online as Service : null;
  // Only a meeting the user picks here changes: an appointment opened and saved keeps its own.
  const [picked, setPicked] = useState(!event);
  const autoTeams = picked && service === "teams" && teamsAuto && !form.link.trim();
  const teams = Boolean(event?.teams) || autoTeams;

  const span = () => {
    if (form.allDay) return { from: wallTime(`${form.day}T00:00`, zone), to: wallTime(`${nextDay(form.lastDay)}T00:00`, zone) };
    const from = wallTime(`${form.day}T${form.time}`, zone);
    return { from, to: form.longEnd ? wallTime(form.longEnd, zone) : new Date(from.getTime() + form.minutes * 60000) };
  };
  const preview = (() => { try { return span(); } catch { return null; } })();

  const save = useMutation({
    mutationFn: async () => {
      let from: Date, to: Date;
      try { ({ from, to } = span()); } catch (error) { throw new FormInvalid({ startsAt: error instanceof Error ? error.message : "Choose a valid start." }); }
      if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime()) || to <= from) throw new FormInvalid({ endsAt: "Choose an end after the start." });
      const problem = service ? linkProblem(service, form.link) : "";
      if (problem) throw new FormInvalid({ location: problem });
      if (picked && service === "teams" && !teams && !form.link.trim()) throw new FormInvalid({ location: "Paste the Teams meeting link." });
      const body: Schemas["CalendarEventInput"] = {
        title: form.title, notes: bodyText(message.html).slice(0, 4000), ...(saysSomething(message.html) ? { notesHtml: message.html } : {}), startsAt: from.toISOString(), endsAt: to.toISOString(), timeZone: zone, allDay: form.allDay,
        // A Teams meeting Outlook makes gets its own location from Outlook.
        location: autoTeams ? form.place.trim() : joinLocation(form.place, service ?? "none", form.link), teams,
        attendees: [...form.attendees.map((email) => ({ email })), ...form.optional.map((email) => ({ email, optional: true }))],
      };
      if (event) return unwrap(await api.PUT("/v1/calendar/events/{eventId}", { params: { path: { eventId: event.id } }, body: { ...body, version: event.version } }));
      return unwrap(await api.POST("/v1/calendar/events", { body }));
    },
    onSuccess: onSaved,
  });
  const errors = fieldErrors(save.error);
  const attendeeError = errors.attendees ?? Object.entries(errors).find(([key]) => key.startsWith("attendees"))?.[1];
  const inviting = form.attendees.length + form.optional.length > 0;
  function allDay(value: boolean) { set({ allDay: value, lastDay: form.day, longEnd: null }); }

  return (
    <section className="panel calendar-form composer-panel" role="dialog" aria-modal="true" aria-label={event ? "Edit appointment" : "New appointment"}>
      <Composer label={event ? "Edit appointment" : "New appointment"} onClose={onClose} closing={save.isPending} onSubmit={() => { save.mutate(); }}
        toolbar={<>
          <button className="primary composer-send" type="submit" disabled={save.isPending}>
            {inviting && <SendIcon />}{save.isPending ? "Saving…" : inviting ? (event ? "Send update" : "Send") : "Save"}
          </button>
          <IconButton label="Discard" icon={<TrashIcon />} disabled={save.isPending} onClick={onClose} />
          <span className="composer-sep" aria-hidden="true" />
          <IconButton label="Signature" tip={signature ? (hasSignature(message.html) ? "Remove signature" : "Add signature") : "Write your signature in Settings > Your account"}
            icon={<SignatureIcon />} pressed={hasSignature(message.html)} disabled={!signature} onClick={toggleSignature} />
          <span className="composer-bar-note muted small">{zone}</span>
        </>}
        side={<DayPeek day={new Date(`${form.day}T00:00`)} start={form.allDay ? null : preview?.from ?? null} end={form.allDay ? null : preview?.to ?? null} title={form.title} skip={event?.id} />}>
        <input className="composer-title" aria-label="Title" placeholder="Add a title" autoFocus required maxLength={200} value={form.title}
          aria-invalid={errors.title ? true : undefined} onChange={(e) => { set({ title: e.target.value }); }} />
        {errors.title && <small className="error">{errors.title}</small>}
        <Row icon={<PeopleIcon />} label="People">
          <div className="composer-people">
            <EmailChips emails={form.attendees} disabled={!connection.connected} invalid={Boolean(attendeeError)} onChange={(attendees) => { set({ attendees }); }} />
            {connection.connected && !showOptional && <button type="button" className="link composer-optional" onClick={() => { setShowOptional(true); }}>Optional</button>}
          </div>
          {showOptional && <EmailChips emails={form.optional} optional label="Optional attendees" placeholder="Optional (Cc)" onChange={(optional) => { set({ optional }); }} />}
          {!connection.connected && <small className="composer-hint">Connect Outlook to invite people.</small>}
          {attendeeError && <small className="error">{attendeeError}</small>}
        </Row>
        <Row icon={<TimeIcon />} label="Time">
          <div className="composer-when">
            <input type="date" aria-label="Start date" required value={form.day} onChange={(e) => { set({ day: e.target.value, ...(form.lastDay < e.target.value ? { lastDay: e.target.value } : {}) }); }} />
            {form.allDay ? <>
              <span className="muted">to</span>
              <input type="date" aria-label="End date" required min={form.day} value={form.lastDay} onChange={(e) => { set({ lastDay: e.target.value }); }} />
            </> : <>
              <StartTime value={form.time} onChange={(time) => { set({ time }); }} />
              <span className="muted">to</span>
              {form.longEnd !== null
                ? <input type="datetime-local" aria-label="End" required value={form.longEnd} onChange={(e) => { set({ longEnd: e.target.value }); }} />
                : <EndTime start={form.time} minutes={form.minutes} choices={ENDS} onChange={(m) => { set({ minutes: m }); }} />}
            </>}
            <label className="composer-toggle"><input type="checkbox" checked={form.allDay} onChange={(e) => { allDay(e.target.checked); }} /> All day</label>
          </div>
          {(errors.startsAt || errors.endsAt) && <small className="error">{errors.startsAt ?? errors.endsAt}</small>}
        </Row>
        <Row icon={<VideoIcon />} label="Meeting">
          {event?.teams
            ? <p className="composer-hint">A Microsoft Teams meeting{event.joinUrl && <> · <a href={event.joinUrl} target="_blank" rel="noreferrer">Join</a></>}</p>
            : <MeetingPicker online={form.online} link={form.link} teamsAuto={teamsAuto} onChange={(online, link) => { setPicked(true); set({ online, link }); }} />}
        </Row>
        <Row icon={<PinIcon />} label="Place">
          <input className="composer-plain" aria-label="Location" placeholder="Add a location" maxLength={300} value={form.place}
            onChange={(e) => { set({ place: e.target.value }); }} />
          {errors.location && <small className="error">{errors.location}</small>}
        </Row>
        <Row icon={<NotesIcon />} label="Details" className="composer-notes">
          <ComposeBody html={message.html} resetKey={message.reset} onChange={write} />
          {event?.teams && <small className="composer-hint">Changed notes stay in HiringPlug: Outlook keeps this Teams meeting&apos;s own notes, which hold its join details.</small>}
          {errors.notes && <small className="error">{errors.notes}</small>}
        </Row>
        <ErrorMessage error={Object.keys(errors).length ? null : save.error} />
      </Composer>
    </section>
  );
}

function addDayText(day: string, days: number): string {
  const d = new Date(`${day}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}
