// The dashboard's first list: what needs the recruiter today, most urgent
// first (red: an offer about to lapse; amber: candidates waiting over 7 days
// in one stage; purple: today's interviews).
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { OPEN_STAGES, STAGE } from "../labels";

type Tone = "urgent" | "warn" | "info";
interface Item { key: string; tone: Tone; title: string; detail: string; href: string; go: string; weight?: number }

const SHOWN = 5;
const ORDER: Record<Tone, number> = { urgent: 0, info: 1, warn: 2 }; // a lapsing offer, today's interviews, then who waits

const time = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });

/** Calendar days from today (local) to a YYYY-MM-DD day. */
function daysUntil(day: string, now: number): number {
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  return Math.round((Date.parse(`${day}T00:00:00`) - today.getTime()) / 86_400_000);
}

export function NeedsYouToday({ permissions, jobs, progress }: {
  permissions: Permission[];
  jobs: Schemas["DashboardJob"][];
  progress?: Map<string, Schemas["JobProgress"]>;
}) {
  const [now] = useState(() => Date.now());
  const [all, setAll] = useState(false);
  const offers = useQuery({
    queryKey: ["offers", "sent"],
    queryFn: async () => unwrap(await api.GET("/v1/offers", { params: { query: { status: "sent" } } })).items,
    enabled: permissions.includes("offers:read"),
    retry: false,
  });
  const interviews = useQuery({
    queryKey: ["interviews", "mine", "today", new Date(now).toDateString()],
    queryFn: async () => {
      const start = new Date(now);
      start.setHours(0, 0, 0, 0);
      const end = new Date(start.getTime() + 86_400_000);
      return unwrap(await api.GET("/v1/interviews", { params: { query: { mine: true, from: start.toISOString(), to: end.toISOString() } } })).items;
    },
    enabled: permissions.includes("interviews:read"),
    retry: false,
  });

  const items: Item[] = [];
  for (const o of offers.data ?? []) {
    if (!o.expiresOn) continue;
    const left = daysUntil(o.expiresOn, now);
    if (left < 0 || left > 2) continue;
    items.push({
      key: `offer-${o.id}`, tone: "urgent", href: `#/candidates/${o.candidateId}`, go: "Open",
      title: `Offer to ${o.candidateName} ${left === 0 ? "expires today" : left === 1 ? "expires tomorrow" : "expires in 2 days"}`,
      detail: `${o.jobTitle}, ${o.companyName}`,
    });
  }
  // One line per job: everyone waiting there, and in which stages.
  for (const j of jobs) {
    const p = progress?.get(j.jobId);
    if (!p || p.waiting === 0) continue;
    const where = OPEN_STAGES.filter((st) => p.waitingCounts[st] > 0);
    items.push({
      key: `wait-${j.jobId}`, tone: "warn", href: `#/jobs/${j.jobId}`, go: where.includes("submitted") ? "Chase the client" : "Open job",
      title: `${p.waiting === 1 ? "1 waiting" : `${String(p.waiting)} waiting`} over 7 days`,
      detail: `${j.title}, ${j.companyName} · ${where.map((st) => `${STAGE[st]} ${String(p.waitingCounts[st])}`).join(", ")}`,
      weight: p.waiting,
    });
  }
  for (const i of interviews.data ?? []) {
    if (i.status !== "scheduled") continue;
    items.push({
      key: `iv-${i.id}`, tone: "info", href: `#/candidates/${i.candidateId}`, go: "Details",
      title: `Interview today ${time.format(new Date(i.startsAt))}`, detail: `${i.candidateName}, ${i.jobTitle}`,
    });
  }

  // Most urgent first: lapsing offers, today's interviews, then the jobs with most people waiting.
  items.sort((a, b) => ORDER[a.tone] - ORDER[b.tone] || (b.weight ?? 0) - (a.weight ?? 0));
  const shown = all ? items : items.slice(0, SHOWN);
  return (
    <section className="panel needs-you" aria-label="Needs you today">
      <h3>Needs you today {items.length > 0 && <span className="count-chip">{items.length}</span>}</h3>
      {items.length === 0 && <p className="muted">Nothing urgent today.</p>}
      {shown.map((it) => (
        <a key={it.key} className={`todo ${it.tone}`} href={it.href}>
          <span className="dot" aria-hidden="true" />
          <span className="todo-text"><strong>{it.title}</strong> <span className="muted">· {it.detail}</span></span>
          <span className="todo-go">{it.go} →</span>
        </a>
      ))}
      {items.length > SHOWN && (
        <button type="button" className="link small" aria-expanded={all} onClick={() => { setAll(!all); }}>
          {all ? "Show fewer" : `Show all ${String(items.length)}`}
        </button>
      )}
    </section>
  );
}
