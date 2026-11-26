// Booking links: make one to put in an email, and see a candidate's bookings.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { filledIn } from "../requiredFields";
import { ErrorMessage } from "./ErrorMessage";

type Link = Schemas["BookingLink"];

export const bookingLinksKey = (candidateId: string) => ["booking-links", candidateId] as const;

const STATUS: Record<Schemas["BookingStatus"], string> = { open: "Waiting", booked: "Booked", cancelled: "Cancelled", expired: "Expired" };

/** "Tue 10 Nov, 10:00" in the viewer's time. */
export function slotLabel(iso: string, opts: Intl.DateTimeFormatOptions = {}): string {
  return new Date(iso).toLocaleString("en-GB", { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", ...opts });
}

/** A small form that makes a booking link and hands back the text to insert. */
export function BookingLinkForm({ candidateId, applicationId, onInsert, onCancel }: {
  candidateId: string; applicationId?: string; onInsert: (text: string) => void; onCancel: () => void;
}) {
  const queryClient = useQueryClient();
  const box = useRef<HTMLDivElement>(null);
  const [form, setForm] = useState({ title: "Call with the recruiter", durationMinutes: 30 as Schemas["BookingLinkInput"]["durationMinutes"], location: "" });
  const [ask, setAsk] = useState<Schemas["BookingAsk"][]>([]);
  const toggle = (a: Schemas["BookingAsk"], on: boolean) => { setAsk((now) => (on ? [...now.filter((x) => x !== a), a] : now.filter((x) => x !== a))); };
  /** How the meeting happens: fills Where, and asks for their phone number when we call them. */
  const how = (kind: "teams" | "phone" | "office") => {
    if (kind === "teams") setForm({ ...form, location: "Microsoft Teams: " });
    if (kind === "phone") { setForm({ ...form, location: "Phone call: we will call you" }); toggle("phone", true); }
    if (kind === "office") setForm({ ...form, location: "Our office: " });
  };
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/booking-links", {
      body: { candidateId, applicationId, title: form.title.trim(), durationMinutes: form.durationMinutes, location: form.location.trim() || null, askFor: ask },
    })),
    onSuccess: async (l) => {
      onInsert(`Please pick a time that suits you (${String(l.durationMinutes)} minutes): ${l.url ?? ""}`);
      await queryClient.invalidateQueries({ queryKey: bookingLinksKey(candidateId) });
    },
  });
  return (
    <div ref={box} className="booking-link-form" role="group" aria-label="Booking link">
      <label className="field">Meeting<input required value={form.title} maxLength={120} onChange={(e) => { setForm({ ...form, title: e.target.value }); }} /></label>
      <label className="field">Length
        <select value={form.durationMinutes} onChange={(e) => { setForm({ ...form, durationMinutes: Number(e.target.value) as typeof form.durationMinutes }); }}>
          {[15, 30, 45, 60, 90].map((m) => <option key={m} value={m}>{m} minutes</option>)}
        </select>
      </label>
      <div className="field booking-how" role="group" aria-label="How">
        <span>How</span>
        <span className="row">
          <button type="button" className="chip-button" onClick={() => { how("teams"); }}>Teams</button>
          <button type="button" className="chip-button" onClick={() => { how("phone"); }}>Phone call</button>
          <button type="button" className="chip-button" onClick={() => { how("office"); }}>In person</button>
        </span>
      </div>
      <label className="field">Where (Teams link, phone or address)<input value={form.location} maxLength={500}
        placeholder="Paste your Teams meeting link, or your number" onChange={(e) => { setForm({ ...form, location: e.target.value }); }} /></label>
      <div className="field booking-ask" role="group" aria-label="Ask them for">
        <label className="check"><input type="checkbox" checked={ask.includes("phone")} onChange={(e) => { toggle("phone", e.target.checked); }} /> Ask for their phone number</label>
        <label className="check"><input type="checkbox" checked={ask.includes("email")} onChange={(e) => { toggle("email", e.target.checked); }} /> Ask for their email</label>
      </div>
      <span className="row">
        <button type="button" className="primary" disabled={create.isPending} onClick={() => { if (filledIn(box.current)) create.mutate(); }}>Insert link</button>
        <button type="button" className="link" onClick={onCancel}>Cancel</button>
      </span>
      <ErrorMessage error={create.error} />
    </div>
  );
}

/** A candidate's booking links and bookings. */
export function CandidateBookings({ candidateId }: { candidateId: string }) {
  const queryClient = useQueryClient();
  const links = useQuery({
    queryKey: bookingLinksKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/booking-links", { params: { query: { candidateId } } })).items,
  });
  const cancel = useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/v1/booking-links/{bookingLinkId}/cancel", { params: { path: { bookingLinkId: id } } })),
    onSettled: async () => { await queryClient.invalidateQueries({ queryKey: bookingLinksKey(candidateId) }); },
  });
  if (!links.data?.length) return <ErrorMessage error={links.error} />;
  return (
    <div className="bookings" aria-label="Bookings">
      <h4>Bookings</h4>
      <ul>
        {links.data.map((l: Link) => (
          <li key={l.id}>
            <span className={`badge booking-${l.status}`}>{STATUS[l.status]}</span>
            <span>{l.title} · {l.durationMinutes} min{l.startsAt ? ` · ${slotLabel(l.startsAt)}` : ""}</span>
            <span className="muted small">{l.hostName}</span>
            {(l.givenPhone ?? l.givenEmail) && <span className="small">They gave: {[l.givenPhone, l.givenEmail].filter(Boolean).join(" · ")}</span>}
            {(l.status === "open" || l.status === "booked") && (
              <button type="button" className="link" disabled={cancel.isPending}
                onClick={() => { if (window.confirm(l.status === "booked" ? "Cancel this meeting? Both calendars get a cancellation." : "Cancel this booking link?")) cancel.mutate(l.id); }}>
                Cancel
              </button>
            )}
          </li>
        ))}
      </ul>
      <ErrorMessage error={cancel.error} />
    </div>
  );
}
