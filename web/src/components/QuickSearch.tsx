// The dashboard's four search boxes: candidates by Kano's /find rules
// (several searches split on commas; each a name, an email or a whole phone
// number, optionally "name - company"); companies; contacts; jobs by title,
// location or client. Results show as you type; Enter opens the only match,
// or the full list. For people, names alike are offered too (for typos).
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { candidatesKey, companiesKey, contactsKey, jobsKey } from "../api/keys";
import { JOB_STATUS } from "../labels";

/** One result: where it opens, its name and a line about it. */
export type Hit = { id: string; href: string; name: string; detail: string; status?: Schemas["JobStatus"] };
/** What one search found; a box with several searches shows a block for each, titled. */
type Block = { title?: string; error?: string; note?: string; hits: Hit[]; similar: Hit[]; more: boolean };
type Found = { blocks: Block[]; skipped: number };

const SHOWN = 5;
const line = (...parts: (string | null | undefined)[]) => parts.filter(Boolean).join(" · ");
const one = (hits: Hit[], similar: Hit[] = []): Found => ({ blocks: [{ hits, similar, more: hits.length === SHOWN }], skipped: 0 });

/** Names alike are worth asking for when the words found few people and the query has letters. */
const wantsSimilar = (q: string, found: number) => found < 3 && /\p{L}{3}/u.test(q);

function candidateHit(c: Schemas["CandidateFound"]): Hit {
  const e = c.employer;
  const where = e ? `${line(e.title, e.company)} (${e.current ? "works there now" : "worked there"})` : line(c.currentTitle, c.currentCompany);
  const score = c.score != null ? `${String(Math.round(c.score * 100))}% name match` : null;
  return { id: c.id, href: `#/candidates/${c.id}`, name: c.fullName, detail: line(score, where, c.email ?? c.phone) };
}

async function findCandidates(q: string): Promise<Found> {
  const found = unwrap(await api.GET("/v1/candidates/find", { params: { query: { q } } }));
  const several = found.searches.length > 1;
  return {
    skipped: found.skipped,
    blocks: found.searches.map((s) => ({
      title: several ? s.text : undefined,
      error: s.error ?? undefined, note: s.note ?? undefined, more: s.more,
      hits: s.items.map(candidateHit), similar: s.similar.map(candidateHit),
    })),
  };
}

async function findContacts(q: string): Promise<Found> {
  const get = async (similar: boolean) => unwrap(await api.GET("/v1/contacts", { params: { query: { q, limit: SHOWN, ...(similar ? { similar } : {}) } } })).items
    .map((c): Hit => ({ id: c.id, href: `#/contacts/${c.id}`, name: c.fullName, detail: line(line(c.title, c.companyName), c.email ?? c.mobile ?? c.phone) }));
  const hits = await get(false);
  return one(hits, wantsSimilar(q, hits.length) ? await get(true) : []);
}

async function findCompanies(q: string): Promise<Found> {
  const page = unwrap(await api.GET("/v1/companies", { params: { query: { q, limit: SHOWN } } }));
  return one(page.items.map((c) => ({ id: c.id, href: `#/companies/${c.id}`, name: c.name, detail: line(c.industry, c.city, c.website) })));
}

async function findJobs(q: string): Promise<Found> {
  const page = unwrap(await api.GET("/v1/jobs", { params: { query: { q, limit: SHOWN } } }));
  return one(page.items.map((j) => ({ id: j.id, href: `#/jobs/${j.id}`, name: j.title, detail: line(j.companyName, j.location), status: j.status })));
}

type Box = { label: string; list: string; key: readonly unknown[]; find: (q: string) => Promise<Found>; hint: string };

const BOXES: { permission: Permission; box: Box }[] = [
  { permission: "candidates:read", box: { label: "Candidates", list: "#/candidates", key: candidatesKey, find: findCandidates, hint: "Name, email or phone; commas for several" } },
  { permission: "jobs:read", box: { label: "Jobs", list: "#/jobs", key: jobsKey, find: findJobs, hint: "Title, location or client" } },
  { permission: "companies:read", box: { label: "Companies", list: "#/companies", key: companiesKey, find: findCompanies, hint: "Name or phone" } },
  { permission: "contacts:read", box: { label: "Contacts", list: "#/contacts", key: contactsKey, find: findContacts, hint: "Name, email or phone" } },
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
  const blocks = q && found.data ? found.data.blocks : [];
  const single = blocks.length === 1 ? blocks[0] : undefined;

  return (
    <div className="search-box">
      <form role="search" aria-label={`Search ${box.label.toLowerCase()}`} onSubmit={(e) => {
        e.preventDefault();
        if (!text.trim() || blocks.length > 1) return; // several searches: their results are the answer
        // The only match opens straight away; otherwise the full list, filtered.
        const only = single?.hits.length === 1 && single.similar.length === 0 && q === text.trim() ? single.hits[0] : undefined;
        window.location.hash = only ? only.href : all;
      }}>
        <label>
          <span className="search-label">{box.label}</span>
          <input type="search" value={text} placeholder={box.hint} aria-label={`Search ${box.label.toLowerCase()}`}
            onChange={(e) => { setText(e.target.value); }} />
        </label>
      </form>
      {q !== "" && (
        <div className="search-results" aria-live="polite">
          {found.isPending && <p className="muted small">Searching…</p>}
          {found.isError && <p className="error small">Could not search. Try again.</p>}
          {blocks.map((b, i) => <Results key={i} block={b} label={box.label} q={q} all={single ? all : undefined} />)}
          {found.data && found.data.skipped > 0 && (
            <p className="muted small">{found.data.skipped} more {found.data.skipped === 1 ? "search was" : "searches were"} left out: six at a time.</p>
          )}
        </div>
      )}
    </div>
  );
}

/** One search's results: its matches, then names alike. */
function Results({ block, label, q, all }: { block: Block; label: string; q: string; all?: string }) {
  const shown = new Set(block.hits.map((h) => h.id));
  const similar = block.similar.filter((h) => !shown.has(h.id));
  const asked = block.title ?? q;
  const name = block.title ? `${label}: ${block.title}` : label;
  return (
    <div className={block.title ? "search-block" : undefined}>
      {block.title && <p className="search-block-title">{block.title}</p>}
      {block.error ? <p className="search-error small">{block.error}</p> : (
        <>
          {block.hits.length === 0 && similar.length === 0 && <p className="muted small">No {label.toLowerCase()} match “{asked}”.</p>}
          <HitList hits={block.hits} label={name} />
          {similar.length > 0 && (
            <>
              <p className="search-similar">Similar names</p>
              <HitList hits={similar} label={`Similar ${block.title ? name : label.toLowerCase()}`} />
            </>
          )}
          {block.note && <p className="search-note small">{block.note}</p>}
          {block.more && all && <a className="search-all" href={all}>See all {label.toLowerCase()} matching “{q}” →</a>}
        </>
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
