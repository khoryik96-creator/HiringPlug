// Schedule or change an interview, composed as an Outlook meeting: Send (with
// the Outlook invitation, when the user's Outlook is connected), the title,
// the candidate and interviewers, the time, where it happens (Teams, Google
// Meet, Zoom, another link, in person or by phone), the candidate's
// instructions, and the user's day beside it.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationInterviewsKey, interviewsKey, usersKey } from "../api/keys";
import { formatWhen, INTERVIEW_MODE } from "../labels";
import { IconButton, MailIcon, OutlookIcon, PhoneIcon, TrashIcon } from "./ActionIcons";
import { escapeHtml, plainToHtml } from "./clientEmail/richText";
import { bodyText, ComposeBody, hasSignature, saysSomething, SignatureIcon, TemplatePicker, useMessage } from "./calendar/ComposeBody";
import { BellIcon, Composer, EmailChips, EndTime, lasting, MeetingPicker, NotesIcon, PeopleIcon, PinIcon, Row, SendIcon, StartTime, TimeIcon, VideoIcon } from "./calendar/Composer";
import { localInput } from "./calendar/dates";
import { DayPeek } from "./calendar/DayPeek";
import { joinLocation, linkProblem, type Service, splitLocation, TEAMS_LOCATION } from "./calendar/meeting";
import { FormInvalid, fieldErrors } from "./forms";
import { SaveError } from "./SaveError";

type Interview = Schemas["Interview"];
type Clash = { title: string; startsAt: string; interviewers: string[] };
type Where = Service | "other" | "in_person" | "phone";
const SERVICE_KEYS: string[] = ["teams", "google_meet", "zoom"];
const LENGTHS = [15, 30, 45, 60, 75, 90, 120, 150, 180, 240, 300, 360, 420, 480];
export const connectionKey = ["calendar-connection"] as const;

/** Where an interview happens, read back from its mode and location. */
function whereOf(iv: Interview | undefined): { where: Where | null; link: string; place: string } {
  if (!iv) return { where: null, link: "", place: "" };
  if (iv.mode !== "video") return { where: iv.mode, link: "", place: iv.location ?? "" };
  const { online, link, place } = splitLocation(iv.location);
  if (online !== "none") return { where: online, link, place }; // a room beside the link is kept
  return { where: place ? "other" : null, link: place, place: "" };
}

/** A new interview starts on the next working day at 10:00, as Outlook proposes a time. */
function nextWorkingDay(): Date {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  while (d.getDay() === 0 || d.getDay() === 6) d.setDate(d.getDate() + 1);
  d.setHours(10, 0, 0, 0);
  return d;
}

const initials = (name: string) => name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? "").join("");

export function InterviewForm({ applicationId, interview, onDone, permissions = [], candidateName, candidateId }: {
  applicationId: string; interview?: Interview; onDone: () => void; permissions?: Permission[]; candidateName?: string; candidateId?: string;
}) {
  const queryClient = useQueryClient();
  const users = useQuery({ queryKey: usersKey, queryFn: async () => unwrap(await api.GET("/v1/users")).items, staleTime: 60_000 });
  const usesCalendar = permissions.includes("calendars:use");
  const outlook = useQuery({ queryKey: connectionKey, queryFn: async () => unwrap(await api.GET("/v1/calendar/connection")), enabled: usesCalendar, staleTime: 60_000 });
  const conn = outlook.data;
  // A sent Teams invitation: Outlook moves the meeting but keeps its message (its join details are in it).
  const sentInvite = useQuery({
    queryKey: ["interview-outlook-invitation", interview?.id ?? "", interview?.version ?? 0],
    queryFn: async () => unwrap(await api.GET("/v1/interviews/{interviewId}/outlook-invitation", { params: { path: { interviewId: interview?.id ?? "" } } })),
    enabled: Boolean(interview) && usesCalendar,
  });
  const teamsInviteSent = Boolean(sentInvite.data?.teams) && ["sent", "pending"].includes(sentInvite.data?.status ?? "");
  const canInvite = !interview && usesCalendar && permissions.includes("interviews:schedule") && Boolean(conn?.connected) && conn?.status !== "reconnect";
  const [opened] = useState(() => ({ version: interview?.version ?? 0 }));
  const [begin] = useState(() => localInput(interview ? new Date(interview.startsAt) : nextWorkingDay()));
  const initial = whereOf(interview);
  const [form, setForm] = useState({
    title: interview?.title ?? "First interview", day: begin.slice(0, 10), time: begin.slice(11, 16),
    durationMinutes: interview?.durationMinutes ?? 60, where: initial.where, link: initial.link, place: initial.place,
    interviewerIds: interview?.interviewers.map((i) => i.userId) ?? [],
  });
  const [invite, setInvite] = useState({ send: true, reminderMinutes: 15, candidateReminder: false, optional: [] as string[] });
  // The message to the candidate, formatted: the invitation's body, and (as text) the interview's instructions.
  const [templated, setTemplated] = useState(false); // a template wrote the message: sent as the invitation, not kept as the instructions
  const [unfilled, setUnfilled] = useState<string[]>([]); // the template's fields still to fill in
  const { message, signature, toggleSignature, replace, write } = useMessage(interview?.instructions ? plainToHtml(interview.instructions) : "", !interview, usesCalendar);
  const who = candidateId ?? interview?.candidateId;
  const [notSent, setNotSent] = useState<string | null>(null);
  const [now] = useState(() => Date.now());
  const set = (change: Partial<typeof form>) => { setForm((f) => ({ ...f, ...change })); };
  const teamsAuto = Boolean(conn?.teamsAvailable);
  const where: Where = form.where ?? (teamsAuto ? "teams" : "google_meet");
  const service = SERVICE_KEYS.includes(where) ? where as Service : null;
  const sending = canInvite && invite.send;
  const start = form.day ? new Date(`${form.day}T${form.time}`) : null;
  const end = start && !Number.isNaN(start.getTime()) ? new Date(start.getTime() + form.durationMinutes * 60000) : null;

  const save = useMutation({
    mutationFn: async (ignoreClashes: boolean) => {
      if (!start || Number.isNaN(start.getTime())) throw new FormInvalid({ startsAt: "Choose the date." });
      if (form.interviewerIds.length === 0) throw new FormInvalid({ interviewerIds: "Add at least one interviewer." });
      const left = unfilled.filter((t) => message.html.includes(escapeHtml(t)));
      if (left.length) throw new FormInvalid({ message: `Fill in ${left.join(", ")} first: the candidate would see it as written.` });
      const problem = service ? linkProblem(service, form.link) : "";
      if (problem) throw new FormInvalid({ location: problem });
      // A Teams meeting with no link needs Outlook to make it (this invitation), or already had one made.
      const outlookMakesTeams = sending && teamsAuto;
      if ((form.where !== null || !interview) && where === "teams" && !form.link.trim() && !outlookMakesTeams && interview?.location !== TEAMS_LOCATION) {
        throw new FormInvalid({ location: "Paste the Teams meeting link, or send the Outlook invitation to make one." });
      }
      const room = form.place.trim();
      const location = form.where === null && interview ? interview.location ?? "" // not chosen here: kept as it was
        : where === "in_person" || where === "phone" ? room
        : where === "other" ? [room, form.link.trim()].filter(Boolean).join("; ")
        : joinLocation(room, where, form.link);
      const mode: Schemas["InterviewMode"] = where === "in_person" || where === "phone" ? where : "video";
      const body = {
        title: form.title, startsAt: start.toISOString(), durationMinutes: form.durationMinutes,
        mode,
        location: location || null, instructions: templated && sending ? null : bodyText(message.html).slice(0, 2000) || null, interviewerIds: form.interviewerIds, ignoreClashes,
      };
      const saved = interview
        ? unwrap(await api.PATCH("/v1/interviews/{interviewId}", { params: { path: { interviewId: interview.id } }, body: { version: opened.version, ...body } }))
        : unwrap(await api.POST("/v1/applications/{applicationId}/interviews", { params: { path: { applicationId } }, body }));
      if (!sending) return null;
      try { // The interview stands either way: only the invitation can fail here.
        unwrap(await api.POST("/v1/interviews/{interviewId}/outlook-invitation", {
          params: { path: { interviewId: saved.id } },
          body: { version: saved.version, teams: where === "teams" && !form.link.trim() && teamsAuto, reminderMinutes: invite.reminderMinutes,
            ...(invite.candidateReminder ? { candidateReminder: true } : {}),
            ...(saysSomething(message.html) ? { bodyHtml: message.html } : {}), ...(invite.optional.length ? { optional: invite.optional } : {}) },
        }));
        return null;
      } catch (e) {
        return e instanceof Error ? e.message : "Outlook could not be reached.";
      }
    },
    onSuccess: async (problem) => {
      await queryClient.invalidateQueries({ queryKey: interviewsKey });
      await queryClient.invalidateQueries({ queryKey: applicationInterviewsKey(applicationId) });
      await queryClient.invalidateQueries({ queryKey: ["applications"] }); // the stage may have moved to interview
      await queryClient.invalidateQueries({ queryKey: ["calendar"] });
      if (problem) setNotSent(problem); else onDone();
    },
  });
  // Any clash can be scheduled over: other interviews (named), calendars busy
  // then, and calendars whose Outlook has not synced lately.
  const clash = save.error instanceof ApiError && save.error.code === "INTERVIEW_CLASH" ? save.error.details : null;
  const clashes = clash ? ((clash.clashes as Clash[] | undefined) ?? []) : null;
  const calendarBusy = (clash?.calendarBusy as string[] | undefined) ?? [];
  const calendarStale = (clash?.calendarStale as string[] | undefined) ?? [];
  const errors = fieldErrors(save.error);
  const people = users.data ?? [];
  const nameOf = (id: string) => people.find((u) => u.id === id)?.name ?? interview?.interviewers.find((i) => i.userId === id)?.name ?? "…";
  const chosen = form.interviewerIds.map((id) => ({ id, name: nameOf(id) }));
  const addable = people.filter((u) => u.canInterview && !form.interviewerIds.includes(u.id));
  const later = start ? start.getTime() > now + 3600000 : false;

  if (notSent) {
    return (
      <section className="panel composer-panel interview-compose" aria-label="Interview scheduled">
        <div className="composer-body">
          <p className="composer-saved" role="status">The interview is scheduled.</p>
          <p className="warning" role="alert">The Outlook invitation was not sent: {notSent} You can send it from the interview.</p>
          <div className="row"><button type="button" className="primary" onClick={onDone}>Close</button></div>
        </div>
      </section>
    );
  }
  return (
    <section className="panel composer-panel interview-compose">
      <Composer label={interview ? "Change interview" : "Schedule interview"} onClose={onDone} closing={save.isPending} onSubmit={() => { save.mutate(false); }}
        toolbar={<>
          <button type="submit" className="primary composer-send" disabled={save.isPending}>
            {sending && <SendIcon />}{save.isPending ? "Saving…" : interview ? "Save" : sending ? "Send" : "Schedule"}
          </button>
          <IconButton label="Discard" icon={<TrashIcon />} disabled={save.isPending} onClick={onDone} />
          {canInvite && <>
            <span className="composer-sep" aria-hidden="true" />
            <IconButton label="Outlook invitation" tip={invite.send ? "Sending the Outlook invitation (click to schedule without it)" : "Send the Outlook invitation too"}
              icon={<OutlookIcon />} pressed={invite.send} onClick={() => { setInvite({ ...invite, send: !invite.send }); }} />
            {invite.send && <label className="composer-tool"><BellIcon />
              <select aria-label="Reminder" value={invite.reminderMinutes} onChange={(e) => { setInvite({ ...invite, reminderMinutes: Number(e.target.value) }); }}>
                <option value={0}>No reminder</option><option value={15}>15 minutes before</option><option value={30}>30 minutes before</option>
                <option value={60}>1 hour before</option><option value={1440}>1 day before</option>
              </select></label>}
            {invite.send && later && <IconButton label="Email candidate a day before" icon={<MailIcon />} pressed={invite.candidateReminder}
              onClick={() => { setInvite({ ...invite, candidateReminder: !invite.candidateReminder }); }} />}
          </>}
          <span className="composer-sep" aria-hidden="true" />
          {who && permissions.includes("emails:send") && <TemplatePicker candidateId={who} applicationId={applicationId}
            words={{ when: start && !Number.isNaN(start.getTime()) ? start.toLocaleString(undefined, { weekday: "long", day: "numeric", month: "long", hour: "numeric", minute: "2-digit" }) : "",
              length: lasting(form.durationMinutes), how: INTERVIEW_MODE[where === "in_person" || where === "phone" ? where : "video"],
              where: where === "in_person" || where === "phone" ? form.place : form.link || (where === "teams" ? "Microsoft Teams" : "") }}
            onPick={(d) => { set({ title: d.subject.slice(0, 100) || form.title }); setTemplated(true); setUnfilled(d.missing); replace(d.html); }} />}
          <IconButton label="Signature" tip={signature ? (hasSignature(message.html) ? "Remove signature" : "Add signature") : "Write your signature in Settings > Your account"}
            icon={<SignatureIcon />} pressed={hasSignature(message.html)} disabled={!signature} onClick={toggleSignature} />
        </>}
        side={usesCalendar && start && !Number.isNaN(start.getTime()) ? <DayPeek day={start} start={start} end={end} title={form.title} skip={interview?.id} /> : undefined}>
        <input className="composer-title" aria-label="Title" placeholder="Add a title" required maxLength={100} value={form.title} onChange={(e) => { set({ title: e.target.value }); }} />
        <Row icon={<PeopleIcon />} label="People">
          <div className="chips-input">
            <span className="person-chip required" title="The candidate (invited)"><span className="avatar">{initials(candidateName ?? interview?.candidateName ?? "C")}</span>
              {candidateName ?? interview?.candidateName ?? "The candidate"} <small>candidate</small></span>
            {chosen.map((u) => (
              <span key={u.id} className="person-chip"><span className="avatar">{initials(u.name)}</span>{u.name}
                <button type="button" aria-label={`Remove ${u.name}`} onClick={() => { set({ interviewerIds: form.interviewerIds.filter((x) => x !== u.id) }); }}>×</button>
              </span>
            ))}
            {addable.length > 0 && (
              <select aria-label="Add interviewer" value="" onChange={(e) => { if (e.target.value) set({ interviewerIds: [...form.interviewerIds, e.target.value] }); }}>
                <option value="">+ Add interviewer</option>
                {addable.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
              </select>
            )}
          </div>
          {sending && <EmailChips emails={invite.optional} optional label="Optional attendees" placeholder="Optional (Cc), e.g. the hiring manager"
            onChange={(optional) => { setInvite({ ...invite, optional }); }} />}
          {errors.interviewerIds && <small className="error" role="alert">{errors.interviewerIds}</small>}
        </Row>
        <Row icon={<TimeIcon />} label="Time">
          <div className="composer-when">
            <input type="date" aria-label="Date" required value={form.day} onChange={(e) => { set({ day: e.target.value }); }} />
            <StartTime value={form.time} onChange={(time) => { set({ time }); }} />
            <span className="muted">to</span>
            <EndTime start={form.time} minutes={form.durationMinutes} choices={LENGTHS} onChange={(m) => { set({ durationMinutes: m }); }} />
          </div>
          {errors.startsAt && <small className="error">{errors.startsAt}</small>}
          {teamsInviteSent && interview && start && (start.getTime() !== Date.parse(interview.startsAt) || form.durationMinutes !== interview.durationMinutes) && (
            <small className="composer-hint" role="note">Outlook moves the Teams meeting and tells the candidate; the invitation&apos;s message stays as it was first sent.</small>
          )}
        </Row>
        <Row icon={<VideoIcon />} label="Where">
          <MeetingPicker online={where} link={form.link} teamsAuto={teamsAuto && (Boolean(interview) || sending)}
            onChange={(online, link) => { set({ where: online as Where, link }); }}
            extra={[{ value: "other", label: "Other link" }, { value: "in_person", label: "In person", icon: <PinIcon /> }, { value: "phone", label: "Phone", icon: <PhoneIcon /> }]} />
          {where === "other" && <input type="url" aria-label="Meeting link" placeholder="Paste the meeting link" maxLength={500} value={form.link} onChange={(e) => { set({ link: e.target.value }); }} />}
          {(where === "in_person" || where === "phone") && (
            <input aria-label={where === "phone" ? "Phone number or note" : "Address"} placeholder={where === "phone" ? "Phone number or note" : "Add the address"}
              maxLength={500} value={form.place} onChange={(e) => { set({ place: e.target.value }); }} />
          )}
          {errors.location && <small className="error" role="alert">{errors.location}</small>}
        </Row>
        <Row icon={<NotesIcon />} label="Details" className="composer-notes">
          <ComposeBody html={message.html} resetKey={message.reset} onChange={write} />
          {errors.message && <small className="error" role="alert">{errors.message}</small>}
        </Row>
        {clashes ? (
          <div className="warning" role="alert">
            <p>Already busy then:</p>
            <ul>
              {clashes.map((c, i) => <li key={i}>{c.interviewers.join(", ")}: {c.title}, {formatWhen(c.startsAt)}</li>)}
              {calendarBusy.map((name) => <li key={`busy-${name}`}>{name}: an appointment in their calendar</li>)}
              {calendarStale.map((name) => <li key={`stale-${name}`}>{name}: their Outlook has not synced lately, so they may be busy</li>)}
            </ul>
            <button type="button" onClick={(e) => { if (e.currentTarget.form?.reportValidity()) save.mutate(true); }}>{sending ? "Send anyway" : "Schedule anyway"}</button>
          </div>
        ) : (
          <SaveError error={save.error} shown={["interviewerIds", "location", "startsAt", "message"]} onReload={onDone} />
        )}
      </Composer>
    </section>
  );
}
