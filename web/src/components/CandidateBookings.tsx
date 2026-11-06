// Booking links: make one to put in an email, and see a candidate's bookings.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
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
  const [form, setForm] = useState({ title: "Call with the recruiter", durationMinutes: 30 as Schemas["BookingLinkInput"]["durationMinutes"], location: "" });
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/booking-links", {
      body: { candidateId, applicationId, title: form.title.trim(), durationMinutes: form.durationMinutes, location: form.location.trim() || null },
    })),
    onSuccess: async (l) => {
      onInsert(`Please pick a time that suits you (${String(l.durationMinutes)} minutes): ${l.url ?? ""}`);
      await queryClient.invalidateQueries({ queryKey: bookingLinksKey(candidateId) });
    },
  });
  return (
    <div className="booking-link-form" role="group" aria-label="Booking link">
      <label className="field">Meeting<input value={form.title} maxLength={120} onChange={(e) => { setForm({ ...form, title: e.target.value }); }} /></label>
      <label className="field">Length
        <select value={form.durationMinutes} onChange={(e) => { setForm({ ...form, durationMinutes: Number(e.target.value) as typeof form.durationMinutes }); }}>
          {[15, 30, 45, 60, 90].map((m) => <option key={m} value={m}>{m} minutes</option>)}
        </select>
      </label>
      <label className="field">Where (link, phone or address)<input value={form.location} maxLength={500} onChange={(e) => { setForm({ ...form, location: e.target.value }); }} /></label>
      <span className="row">
        <button type="button" className="primary" disabled={create.isPending || !form.title.trim()} onClick={() => { create.mutate(); }}>Insert link</button>
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
