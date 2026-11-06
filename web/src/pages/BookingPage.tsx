// The public booking page (no sign-in): a candidate picks a time from a
// recruiter's free times, in their own time zone. Lives at /book/<token>.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";

type Booking = Schemas["PublicBooking"];

const ROUTE = /^\/book\/([A-Za-z0-9_-]{32,64})\/?$/;

/** The token of a booking page address (/book/<token>, or #/book/<token>). */
export function bookingRoute(loc: Pick<Location, "pathname" | "hash">): string | null {
  return ROUTE.exec(loc.hash.replace(/^#/, ""))?.[1] ?? ROUTE.exec(loc.pathname)?.[1] ?? null;
}

const zone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;
const dayKey = (iso: string) => new Date(iso).toLocaleDateString("en-CA"); // YYYY-MM-DD, local
const dayLabel = (iso: string) => new Date(iso).toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short" });
const timeLabel = (iso: string) => new Date(iso).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
const fullLabel = (iso: string) => new Date(iso).toLocaleString("en-GB", { weekday: "long", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });

/** The free times by day, in the visitor's time. */
export function byDay(slots: string[]): [string, string[]][] {
  const days = new Map<string, string[]>();
  for (const s of slots) days.set(dayKey(s), [...(days.get(dayKey(s)) ?? []), s]);
  return [...days.entries()];
}

export function BookingPage({ token }: { token: string }) {
  const queryClient = useQueryClient();
  const key = ["public-booking", token];
  const page = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/public/bookings/{bookingToken}", { params: { path: { bookingToken: token } } })),
  });
  const [changing, setChanging] = useState(false);
  const book = useMutation({
    mutationFn: async (startsAt: string) => unwrap(await api.POST("/v1/public/bookings/{bookingToken}", {
      params: { path: { bookingToken: token } }, body: { startsAt },
    })),
    onSuccess: (b) => { setChanging(false); queryClient.setQueryData(key, b); },
    onError: async () => { await queryClient.invalidateQueries({ queryKey: key }); }, // a taken time leaves the list
  });
  const b = page.data;
  return (
    <div className="booking-page">
      <main className="booking-card">
        {!b ? (page.error ? <ErrorMessage error={page.error} /> : <p className="muted">Loading…</p>) : (
          <>
            <p className="booking-company">{b.companyName}</p>
            <h1>{b.title}</h1>
            <p className="muted">with {b.hostName} · {b.durationMinutes} minutes</p>
            {b.status === "booked" && !changing && <Booked b={b} onChange={() => { setChanging(true); }} />}
            {b.status === "cancelled" && <p className="booking-note">This meeting was cancelled. Ask {b.hostName} for a new link if you still want to meet.</p>}
            {b.status === "expired" && <p className="booking-note">This link has expired. Ask {b.hostName} for a new one.</p>}
            {(b.status === "open" || changing) && (
              <Picker b={b} busy={book.isPending} onPick={(s) => { book.mutate(s); }} onCancel={changing ? () => { setChanging(false); } : undefined} />
            )}
            <ErrorMessage error={book.error} />
          </>
        )}
      </main>
    </div>
  );
}

function Booked({ b, onChange }: { b: Booking; onChange: () => void }) {
  return (
    <div className="booking-done" role="status">
      <p className="booking-check">You&apos;re booked</p>
      <p><strong>{b.startsAt ? fullLabel(b.startsAt) : ""}</strong> <span className="muted">({zone()})</span></p>
      {b.location && <p>Where: {/^https?:\/\//.test(b.location) ? <a href={b.location} target="_blank" rel="noreferrer noopener">{b.location}</a> : b.location}</p>}
      <p className="muted small">An invitation is on its way to your email: add it to your calendar.</p>
      {b.canChange && <button type="button" className="link" onClick={onChange}>Change the time</button>}
    </div>
  );
}

function Picker({ b, busy, onPick, onCancel }: { b: Booking; busy: boolean; onPick: (s: string) => void; onCancel?: () => void }) {
  const days = byDay(b.slots);
  const [picked, setDay] = useState(days[0]?.[0] ?? "");
  const [choice, setChosen] = useState<string | null>(null);
  if (days.length === 0) return <p className="booking-note">No free times are left. Ask {b.hostName} for another link.</p>;
  // After a refresh (someone took a time), a day or time no longer offered is not kept.
  const day = days.some(([d]) => d === picked) ? picked : (days[0]?.[0] ?? "");
  const times = days.find(([d]) => d === day)?.[1] ?? [];
  const chosen = choice !== null && times.includes(choice) ? choice : null;
  return (
    <>
      <h2 className="booking-step">Pick a day</h2>
      <div className="booking-days" role="radiogroup" aria-label="Day">
        {days.map(([d, list]) => (
          <button key={d} type="button" role="radio" aria-checked={d === day} className={d === day ? "on" : undefined}
            onClick={() => { setDay(d); setChosen(null); }}>
            <span>{dayLabel(list[0] ?? d)}</span><small>{list.length} times</small>
          </button>
        ))}
      </div>
      <h2 className="booking-step">Pick a time <span className="muted small">({zone().replace(/_/g, " ")})</span></h2>
      <div className="booking-times" role="radiogroup" aria-label="Time">
        {times.map((t) => (
          <button key={t} type="button" role="radio" aria-checked={t === chosen} className={t === chosen ? "on" : undefined}
            onClick={() => { setChosen(t); }}>{timeLabel(t)}</button>
        ))}
      </div>
      <div className="row">
        <button type="button" className="primary" disabled={!chosen || busy} onClick={() => { if (chosen) onPick(chosen); }}>
          {chosen ? `Book ${dayLabel(chosen)}, ${timeLabel(chosen)}` : "Pick a time"}
        </button>
        {onCancel && <button type="button" className="link" onClick={onCancel}>Keep the booked time</button>}
      </div>
    </>
  );
}
