import { useMutation } from "@tanstack/react-query";
import { api, type Schemas, unwrap } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import { ServiceMark } from "./Composer";
import { dateLabel, timeLabel } from "./dates";
import { joinOf, SHORT_LABEL } from "./meeting";

export function EventDetails({ event, onClose, onEdit, onChanged }: { event: Schemas["CalendarEvent"]; onClose: () => void; onEdit: () => void; onChanged: () => void }) {
  const join = joinOf(event.location);
  const action = useMutation({ mutationFn: async (kind: "cancel" | "retry") => {
    const path = kind === "cancel" ? "/v1/calendar/events/{eventId}/cancel" : "/v1/calendar/events/{eventId}/retry";
    return unwrap(await api.POST(path, { params: { path: { eventId: event.id } }, body: { version: event.version } }));
  }, onSuccess: onChanged });
  return <section className="panel calendar-detail" role="dialog" aria-modal="true" aria-label="Appointment details">
    <div className="calendar-heading"><span className="badge">{event.source === "local" ? "Appointment" : event.source === "interview_invitation" ? "Interview invitation" : event.source}</span><button autoFocus onClick={onClose}>Close</button></div>
    <h3>{event.title}</h3>{event.status === "cancelled" && <p className="warning">Cancelled in HiringPlug; Outlook cancellation is awaiting confirmation.</p>}<p>{dateLabel(new Date(event.startsAt))} · {event.allDay ? "All day" : `${timeLabel(event.startsAt)} – ${timeLabel(event.endsAt)}`}</p>
    {event.location && <p>{event.location}</p>}
    {!event.joinUrl && join && <p><a className="button primary join-link" href={join.url} target="_blank" rel="noreferrer"><ServiceMark service={join.service} /> Join {SHORT_LABEL[join.service]}</a></p>}
    {event.notes && <p className="calendar-notes">{event.notes}</p>}
    {event.attendees.length > 0 && <p className="muted small">{event.attendees.map((a) => a.name ? `${a.name} <${a.email}>` : a.email).join(", ")}</p>}
    {event.syncStatus === "pending" && <p role="status" className="muted">Waiting to sync with Outlook.</p>}
    {event.syncError && <p className="warning" role="alert">{event.syncError}</p>}
    <ErrorMessage error={action.error}/>
    <div className="row">
      {event.editable && <button onClick={onEdit}>Edit</button>}
      {event.joinUrl && <a className="button primary" href={event.joinUrl} target="_blank" rel="noreferrer">Join meeting</a>}
      {event.webUrl && <a className="button" href={event.webUrl} target="_blank" rel="noreferrer">Open in Outlook</a>}
      {(event.syncStatus === "failed" || event.syncStatus === "conflict") && event.source !== "outlook" && event.source !== "interview_invitation" && <button disabled={action.isPending} onClick={() => { if (window.confirm("Keep the HiringPlug details and sync them to Outlook? Any attendees will receive an update.")) action.mutate("retry"); }}>Keep HiringPlug details & sync</button>}
      {event.editable && <button disabled={action.isPending} onClick={() => { if (window.confirm(event.attendees.length ? "Cancel this appointment and send cancellations to its attendees?" : "Cancel this appointment?")) action.mutate("cancel"); }}>Cancel appointment</button>}
    </div>
    {(event.source === "interview" || event.source === "booking" || event.source === "interview_invitation") && <p className="small muted">Change this meeting in its HiringPlug interview or booking link.</p>}
  </section>;
}
