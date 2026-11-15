// Search results read as resumes: the results down the left, the chosen
// candidate's full CV on the right (with the search's words marked). Next and
// Prev (or the ← → keys) move through the results without leaving the page.
import { type ReactNode, useEffect, useRef, useState } from "react";

import type { Schemas } from "../../api/client";
import { jobsKey } from "../../api/keys";
import { formatNotice } from "../../labels";
import { searchOpenJobs, useAddToJob } from "../addToJob";
import { CandidateCv } from "../CandidateCv";
import { WaDisc, workAuth } from "../candidateColumns";
import { rangeText, salaryText, waColumnLabel } from "../candidateLook";
import { ErrorMessage } from "../ErrorMessage";
import { type Picked, SearchPicker } from "../SearchPicker";

type Hit = Schemas["CandidateSearchHit"];
type Summary = Schemas["CandidateSummary"];

/**
 * Reads the results as resumes. Give it a new `key` for each new search, so it
 * starts again at the first result (and, on a phone, at the list).
 */
export function ResumeReader({ hits, highlight, seen, onSeen, chosen, onChoose, canAddToJob, more, onMore }: {
  hits: Hit[];
  /** The boolean search whose words are marked in the CV (and on the profile it opens; "" for none). */
  highlight: string;
  /** Candidates already looked at in this search. */
  seen: ReadonlySet<string>;
  onSeen: (id: string) => void;
  /** People chosen to email (when the person may email). */
  chosen?: ReadonlySet<string>;
  onChoose?: (c: Summary, on: boolean) => void;
  canAddToJob: boolean;
  /** "Show more", under the list. */
  more?: ReactNode;
  /** Loads the next page of results (when there is one): Next goes on into it. */
  onMore?: () => void;
}) {
  const [openId, setOpenId] = useState<string | null>(null);
  const [reading, setReading] = useState(false); // on a phone: the resume fills the screen
  const pane = useRef<HTMLElement>(null);
  const [addingFor, setAddingFor] = useState<string | null>(null); // the candidate being added to a job
  const [nextWhenLoaded, setNextWhenLoaded] = useState<string | null>(null); // Next was pressed on this last result
  const profileQuery = highlight ? `?hl=${encodeURIComponent(highlight)}` : "";
  // Once the next page is in, Next carries on into it.
  const after = nextWhenLoaded ? hits.findIndex((h) => h.candidate.id === nextWhenLoaded) + 1 : 0;
  const waitingFor = after > 0 ? hits[after]?.candidate.id : undefined;
  const shownId = waitingFor ?? openId;
  const loading = nextWhenLoaded !== null && !waitingFor;
  const at = Math.max(0, hits.findIndex((h) => h.candidate.id === shownId));
  const current = hits[at];
  const currentId = current?.candidate.id;
  const open = (i: number, focus = false) => {
    const h = hits[i];
    if (!h) return;
    setOpenId(h.candidate.id);
    setNextWhenLoaded(null);
    const el = document.getElementById(hitId(h.candidate.id));
    if (el) {
      reveal(el);
      if (focus) el.focus();
    }
  };

  const last = at === hits.length - 1;
  const next = () => {
    if (!last) open(at + 1);
    else if (onMore && currentId) { setNextWhenLoaded(currentId); onMore(); } // again if a load failed
  };
  useEffect(() => { if (currentId) onSeen(currentId); }, [currentId, onSeen]);
  // ← and → move through the results, unless the person is typing, selecting
  // (Shift) or another control already used the key.
  const keys = useRef<(e: KeyboardEvent) => void>(() => undefined);
  useEffect(() => {
    keys.current = (e) => {
      if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey || typing(e.target)) return;
      if (e.key === "ArrowRight") { e.preventDefault(); next(); }
      if (e.key === "ArrowLeft") { e.preventDefault(); open(at - 1); }
    };
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { keys.current(e); };
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("keydown", onKey); };
  }, []);

  if (!current) return null;
  return (
    <div className={`reader${reading ? " reading" : ""}`}>
      <div className="reader-list">
        <ul aria-label="Results" onKeyDown={(e) => {
          if (e.key === "ArrowDown") { e.preventDefault(); if (last) next(); else open(at + 1, true); }
          if (e.key === "ArrowUp") { e.preventDefault(); open(at - 1, true); }
        }}>
          {hits.map(({ candidate: c, matchedIn }) => (
            <li key={c.id}>
              <button type="button" id={hitId(c.id)} className={`reader-hit${c.id === currentId ? " on" : ""}`}
                aria-current={c.id === currentId ? "true" : undefined}
                onClick={() => {
                  setOpenId(c.id);
                  setNextWhenLoaded(null);
                  setReading(true);
                  // On a phone the resume replaces the list: start at its top.
                  if (narrow()) requestAnimationFrame(() => { if (pane.current) reveal(pane.current, "start"); });
                }}>
                <WaDisc c={c} />
                <span className="who">
                  <span className="nm">{c.fullName}</span>
                  <span className="ln">{[c.currentTitle, c.currentCompany].filter(Boolean).join(" · ") || "—"}</span>
                  <span className="ln">{payLine(c)}</span>
                </span>
                <span className="tags">
                  {matchedIn.length > 0 && <span className="found-in">{matchedIn.includes("cv") ? "in CV" : "in profile"}</span>}
                  {chosen?.has(c.id) && <span className="chosen-mark">✉ chosen</span>}
                  {seen.has(c.id) && c.id !== currentId && <span className="seen">✓ read</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {more}
      </div>
      <section ref={pane} className="reader-pane" aria-label={`Resume of ${current.candidate.fullName}`}>
        <button type="button" className="link reader-back" onClick={() => {
          setReading(false);
          requestAnimationFrame(() => { const el = document.getElementById(hitId(current.candidate.id)); if (el) reveal(el, "center"); });
        }}>‹ Back to results</button>
        <PaneHead c={current.candidate} />
        <div className="pane-bar">
          <button type="button" className="pill-btn" disabled={at === 0} onClick={() => { open(at - 1); }}>‹ Prev</button>
          <span role="status">{String(at + 1)} of {String(hits.length)}</span>
          <button type="button" className="pill-btn" disabled={last && !onMore} onClick={next}>
            {last && onMore ? (loading ? "Loading…" : "Load more ›") : "Next ›"}
          </button>
          <span className="muted small keys"><kbd>←</kbd> <kbd>→</kbd> to move</span>
          <span className="sp" />
          <a className="pill-btn" href={`#/candidates/${current.candidate.id}${profileQuery}`}>Open profile</a>
          {canAddToJob && (
            <button type="button" className="pill-btn" aria-expanded={addingFor === currentId}
              onClick={() => { setAddingFor(addingFor === currentId ? null : (currentId ?? null)); }}>+ Add to job</button>
          )}
          {onChoose && chosen && (
            <button type="button" className={`pill-btn${chosen.has(current.candidate.id) ? " primary" : ""}`} aria-pressed={chosen.has(current.candidate.id)}
              onClick={() => { onChoose(current.candidate, !chosen.has(current.candidate.id)); }}>
              {chosen.has(current.candidate.id) ? "✓ Chosen to email" : "Choose to email"}
            </button>
          )}
        </div>
        {canAddToJob && addingFor === currentId && <AddToJob key={current.candidate.id} candidateId={current.candidate.id} />}
        <div className="reader-cv">
          {/* A new search marks its own words: the CV starts again. */}
          <CandidateCv key={`${current.candidate.id} ${highlight}`} candidateId={current.candidate.id} highlight={highlight} unified
            empty={<p className="muted reader-empty">No CV on file for this candidate yet. Open their profile to add one.</p>} />
        </div>
      </section>
    </div>
  );
}

const hitId = (id: string) => `reader-hit-${id}`;

/** Brings an element into view where the browser can (not in tests). */
function reveal(el: HTMLElement, block: ScrollLogicalPosition = "nearest") {
  if (typeof el.scrollIntoView === "function") el.scrollIntoView({ block });
}

/** A phone-sized screen, where the list and the resume take turns (see search-reader.css). */
const narrow = () => typeof window.matchMedia === "function" && window.matchMedia("(max-width: 820px)").matches;

function typing(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName);
}

/** "RM 8,000 → RM 9,500 · 1 month": pay (when the person may see it) and notice. */
function payLine(c: Summary): string {
  const parts: string[] = [];
  if (!c.salaryRestricted && (c.currentSalary ?? c.expectedSalary)) {
    parts.push(`${c.currentSalary ? salaryText(c.currentSalary)[0] : "?"} → ${c.expectedSalary ? rangeText(c.expectedSalary)[0] : "?"}`);
  }
  if (c.noticePeriod) parts.push(formatNotice(c.noticePeriod));
  return parts.join(" · ") || "—";
}

function PaneHead({ c }: { c: Summary }) {
  const wa = workAuth(c);
  const fact = (label: string, value: [string, string] | string | null) => (
    <span className="fact">
      {label}
      <b>{value === null ? "—" : typeof value === "string" ? value : <>{value[0]} <small>{value[1]}</small></>}</b>
    </span>
  );
  return (
    <div className="pane-head">
      <WaDisc c={c} big />
      <div className="pane-who">
        <h3>{c.fullName}</h3>
        <div className="muted">{[c.currentTitle, c.currentCompany, wa ? waColumnLabel(wa) : null].filter(Boolean).join(" · ")}</div>
      </div>
      <div className="facts">
        {c.salaryRestricted ? fact("Salary", "Restricted") : <>
          {fact("Current", c.currentSalary ? salaryText(c.currentSalary) : null)}
          {fact("Expected", c.expectedSalary ? rangeText(c.expectedSalary) : null)}
        </>}
        {fact("Notice", c.noticePeriod ? formatNotice(c.noticePeriod) : null)}
      </div>
    </div>
  );
}

/** Add the candidate to an open job, from the resume. */
function AddToJob({ candidateId }: { candidateId: string }) {
  const [job, setJob] = useState<Picked | null>(null);
  const add = useAddToJob(candidateId);
  return (
    <div className="pane-bar add-to-job">
      <span className="muted small">Add to a job</span>
      <SearchPicker value={job} onChange={(p) => { setJob(p); add.reset(); }} queryKey={jobsKey}
        placeholder="Type a job title" noMatch="No open job matches." search={searchOpenJobs} />
      {job && !add.isSuccess && <button type="button" className="pill-btn primary" disabled={add.isPending} onClick={() => { add.mutate(job.id); }}>Add</button>}
      {add.isSuccess && job && <span role="status" className="small">✓ Added to {job.name}</span>}
      <ErrorMessage error={add.error} />
    </div>
  );
}
