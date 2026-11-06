// The dashboard's four search boxes (like Kano's /find): candidates by name,
// email, phone, title or company; companies; contacts; jobs by title,
// location or client. Results show as you type; Enter opens the only match,
// or the full list. For people, names alike are offered too (for typos).
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { candidatesKey, companiesKey, contactsKey, jobsKey } from "../api/keys";
import { JOB_STATUS } from "../labels";

/** One result: where it opens, its name and a line about it. */
export type Hit = { id: string; href: string; name: string; detail: string; status?: Schemas["JobStatus"] };
type Found = { hits: Hit[]; similar: Hit[] };

const SHOWN = 5;
const line = (...parts: (string | null | undefined)[]) => parts.filter(Boolean).join(" · ");

/** Names alike are worth asking for when the words found few people and the query has letters. */
const wantsSimilar = (q: string, found: number) => found < 3 && /\p{L}{3}/u.test(q);

async function findCandidates(q: string): Promise<Found> {
  const get = async (similar: boolean) => unwrap(await api.GET("/v1/candidates", { params: { query: { q, limit: SHOWN, ...(similar ? { similar } : {}) } } })).items
    .map((c): Hit => ({ id: c.id, href: `#/candidates/${c.id}`, name: c.fullName, detail: line(line(c.currentTitle, c.currentCompany), c.email ?? c.phone) }));
  const hits = await get(false);
  return { hits, similar: wantsSimilar(q, hits.length) ? await get(true) : [] };
}

async function findContacts(q: string): Promise<Found> {
  const get = async (similar: boolean) => unwrap(await api.GET("/v1/contacts", { params: { query: { q, limit: SHOWN, ...(similar ? { similar } : {}) } } })).items
    .map((c): Hit => ({ id: c.id, href: `#/contacts/${c.id}`, name: c.fullName, detail: line(line(c.title, c.companyName), c.email ?? c.mobile ?? c.phone) }));
  const hits = await get(false);
  return { hits, similar: wantsSimilar(q, hits.length) ? await get(true) : [] };
}

async function findCompanies(q: string): Promise<Found> {
  const page = unwrap(await api.GET("/v1/companies", { params: { query: { q, limit: SHOWN } } }));
  return { hits: page.items.map((c) => ({ id: c.id, href: `#/companies/${c.id}`, name: c.name, detail: line(c.industry, c.city, c.website) })), similar: [] };
}

async function findJobs(q: string): Promise<Found> {
  const page = unwrap(await api.GET("/v1/jobs", { params: { query: { q, limit: SHOWN } } }));
  return {
    hits: page.items.map((j) => ({ id: j.id, href: `#/jobs/${j.id}`, name: j.title, detail: line(j.companyName, j.location), status: j.status })),
    similar: [],
  };
}

type Box = { label: string; placeholder: string; list: string; key: readonly unknown[]; find: (q: string) => Promise<Found> };

const BOXES: { permission: Permission; box: Box }[] = [
  { permission: "candidates:read", box: { label: "Candidates", placeholder: "Name, email, phone, title or company", list: "#/candidates", key: candidatesKey, find: findCandidates } },
  { permission: "jobs:read", box: { label: "Jobs", placeholder: "Title, client or location", list: "#/jobs", key: jobsKey, find: findJobs } },
  { permission: "companies:read", box: { label: "Companies", placeholder: "Name, website or phone", list: "#/companies", key: companiesKey, find: findCompanies } },
  { permission: "contacts:read", box: { label: "Contacts", placeholder: "Name, email, phone or title", list: "#/contacts", key: contactsKey, find: findContacts } },
];

export function QuickSearch({ permissions }: { permissions: Permission[] }) {
  const boxes = BOXES.filter((b) => permissions.includes(b.permission));
  if (boxes.length === 0) return null;
  return (
    <section className="quick-search" aria-label="Search">
      {boxes.map(({ box }) => <SearchBox key={box.label} box={box} />)}
    </section>
  );
}

/** The text typed, once typing pauses (so each key press does not ask the server). */
function useSettled(text: string, ms = 250): string {
  const [settled, setSettled] = useState(text);
  useEffect(() => {
    const timer = setTimeout(() => { setSettled(text); }, ms);
    return () => { clearTimeout(timer); };
  }, [text, ms]);
  return settled;
}

function SearchBox({ box }: { box: Box }) {
  const [text, setText] = useState("");
  const q = useSettled(text.trim());
  const found = useQuery({ queryKey: [...box.key, "quick", q], queryFn: () => box.find(q), enabled: q !== "" });
  const all = `${box.list}?q=${encodeURIComponent(text.trim())}`;
  const hits = q && found.data ? found.data.hits : [];
  const shown = new Set(hits.map((h) => h.id));
  const similar = q && found.data ? found.data.similar.filter((h) => !shown.has(h.id)) : [];

  return (
    <div className="search-box">
      <form role="search" aria-label={`Search ${box.label.toLowerCase()}`} onSubmit={(e) => {
        e.preventDefault();
        if (!text.trim()) return;
        // The only match opens straight away; otherwise the full list, filtered.
        const only = hits.length === 1 && similar.length === 0 && q === text.trim() ? hits[0] : undefined;
        window.location.hash = only ? only.href : all;
      }}>
        <label>
          <span className="search-label">{box.label}</span>
          <input type="search" value={text} placeholder={box.placeholder} aria-label={`Search ${box.label.toLowerCase()}`}
            onChange={(e) => { setText(e.target.value); }} />
        </label>
      </form>
      {q !== "" && (
        <div className="search-results" aria-live="polite">
          {found.isPending && <p className="muted small">Searching…</p>}
          {found.isError && <p className="error small">Could not search. Try again.</p>}
          {found.data && hits.length === 0 && similar.length === 0 && <p className="muted small">No {box.label.toLowerCase()} match “{q}”.</p>}
          <HitList hits={hits} label={box.label} />
          {similar.length > 0 && (
            <>
              <p className="search-similar">Similar names</p>
              <HitList hits={similar} label={`Similar ${box.label.toLowerCase()}`} />
            </>
          )}
          {hits.length === SHOWN && <a className="search-all" href={all}>See all {box.label.toLowerCase()} matching “{q}” →</a>}
        </div>
      )}
    </div>
  );
}

function HitList({ hits, label }: { hits: Hit[]; label: string }) {
  if (hits.length === 0) return null;
  return (
    <ul className="search-hits" aria-label={label}>
      {hits.map((h) => (
        <li key={h.id}>
          <a href={h.href}>
            <span className="search-name">{h.name}{h.status && <span className={`badge ${h.status}`}>{JOB_STATUS[h.status]}</span>}</span>
            {h.detail && <span className="search-detail">{h.detail}</span>}
          </a>
        </li>
      ))}
    </ul>
  );
}
