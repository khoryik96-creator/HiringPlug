// Search results read as resumes: the results down the left, the chosen
// candidate's full CV on the right (with the search's words marked). Next and
// Prev (or the ← → keys) move through the results without leaving the page.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { applicationsKey, jobsKey } from "../../api/keys";
import { formatNotice } from "../../labels";
import { CandidateCv } from "../CandidateCv";
import { initials, workAuth } from "../candidateColumns";
import { rangeText, salaryText, WA_KIND, waColumnLabel, waKind } from "../candidateLook";
import { ErrorMessage } from "../ErrorMessage";
import { type Picked, SearchPicker } from "../SearchPicker";

type Hit = Schemas["CandidateSearchHit"];
type Summary = Schemas["CandidateSummary"];

export function ResumeReader({ hits, highlight, profileQuery, seen, onSeen, chosen, onChoose, canAddToJob, more }: {
  hits: Hit[];
  /** The boolean search whose words are marked in the CV ("" for none). */
  highlight: string;
  /** Added to the profile's link so its CV is marked too ("?hl=…" or ""). */
  profileQuery: string;
  /** Candidates already looked at in this search. */
  seen: ReadonlySet<string>;
  onSeen: (id: string) => void;
  /** People chosen to email (when the person may email). */
  chosen?: ReadonlySet<string>;
  onChoose?: (c: Summary, on: boolean) => void;
  canAddToJob: boolean;
  /** "Show more", under the list. */
  more?: ReactNode;
}) {
  const [openId, setOpenId] = useState<string | null>(null);
  const [reading, setReading] = useState(false); // on a phone: the resume fills the screen
  const pane = useRef<HTMLElement>(null);
  const [addingFor, setAddingFor] = useState<string | null>(null); // the candidate being added to a job
  const at = Math.max(0, hits.findIndex((h) => h.candidate.id === openId));
  const current = hits[at];
  const currentId = current?.candidate.id;
  const open = (i: number, focus = false) => {
    const h = hits[i];
    if (!h) return;
    setOpenId(h.candidate.id);
    const el = document.getElementById(hitId(h.candidate.id));
    if (el) {
      reveal(el);
      if (focus) el.focus();
    }
  };

  useEffect(() => { if (currentId) onSeen(currentId); }, [currentId, onSeen]);
  // ← and → move through the results, unless the person is typing.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.altKey || e.ctrlKey || e.metaKey || typing(e.target)) return;
      if (e.key === "ArrowRight") { e.preventDefault(); open(at + 1); }
      if (e.key === "ArrowLeft") { e.preventDefault(); open(at - 1); }
    };
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("keydown", onKey); };
  });

  if (!current) return null;
  return (
    <div className={`reader${reading ? " reading" : ""}`}>
      <div className="reader-list">
        <ul aria-label="Results" onKeyDown={(e) => {
          if (e.key === "ArrowDown") { e.preventDefault(); open(at + 1, true); }
          if (e.key === "ArrowUp") { e.preventDefault(); open(at - 1, true); }
        }}>
          {hits.map(({ candidate: c, matchedIn }) => (
            <li key={c.id}>
              <button type="button" id={hitId(c.id)} className={`reader-hit${c.id === currentId ? " on" : ""}`}
                aria-current={c.id === currentId ? "true" : undefined}
                onClick={() => {
                  setOpenId(c.id);
                  setReading(true);
                  // On a phone the resume replaces the list: start at its top.
                  if (narrow()) requestAnimationFrame(() => { if (pane.current) reveal(pane.current, "start"); });
                }}>
                <Disc c={c} />
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
          <button type="button" className="pill-btn" disabled={at === hits.length - 1} onClick={() => { open(at + 1); }}>Next ›</button>
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
          <CandidateCv key={current.candidate.id} candidateId={current.candidate.id} highlight={highlight} unified
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

function Disc({ c, big = false }: { c: Summary; big?: boolean }) {
  const kind = WA_KIND[waKind(workAuth(c))];
  return (
    <span className={`wa-disc${big ? " big" : ""}`} aria-hidden="true"
      style={{ background: kind.color, boxShadow: `0 0 0 2px #fff, 0 0 0 4px ${kind.color}` }}>{initials(c.fullName)}</span>
  );
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
      <Disc c={c} big />
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
  const queryClient = useQueryClient();
  const [job, setJob] = useState<Picked | null>(null);
  const add = useMutation({
    mutationFn: async (jobId: string) =>
      unwrap(await api.POST("/v1/jobs/{jobId}/applications", { params: { path: { jobId } }, body: { candidateId } })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: applicationsKey }); // the candidate's jobs too
    },
  });
  return (
    <div className="pane-bar add-to-job">
      <span className="muted small">Add to a job</span>
      <SearchPicker value={job} onChange={(p) => { setJob(p); add.reset(); }} queryKey={jobsKey}
        placeholder="Type a job title" noMatch="No open job matches."
        search={async (q) => unwrap(await api.GET("/v1/jobs", { params: { query: { q, status: "open", limit: 8 } } })).items.map((j) => ({ id: j.id, name: j.title, detail: j.companyName }))} />
      {job && !add.isSuccess && <button type="button" className="pill-btn primary" disabled={add.isPending} onClick={() => { add.mutate(job.id); }}>Add</button>}
      {add.isSuccess && job && <span role="status" className="small">✓ Added to {job.name}</span>}
      <ErrorMessage error={add.error} />
    </div>
  );
}
