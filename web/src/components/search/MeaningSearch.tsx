// Search by meaning on the candidates page: the switch beside the search box,
// how the search was read (and how far the reading of the company's CVs has
// got), and why each candidate was found. The server reads the words in a
// moment; meanwhile the list asks again (useMeaningPolling).
import { useEffect, useState } from "react";

import type { Schemas } from "../../api/client";

const KEY = "hiringplug.candidates.meaning";

/** Whether the search box searches by meaning (kept on this device: a convenience). */
export function useMeaningMode(): [boolean, (on: boolean) => void] {
  const [on, setOn] = useState(() => {
    try { return window.localStorage.getItem(KEY) === "1"; } catch { return false; }
  });
  const set = (v: boolean) => {
    setOn(v);
    try { window.localStorage.setItem(KEY, v ? "1" : "0"); } catch { /* only a convenience */ }
  };
  return [on, set];
}

/** The switch in the search box. */
export function MeaningSwitch({ on, onChange }: { on: boolean; onChange: (on: boolean) => void }) {
  return (
    <button type="button" className={`search-pill meaning-pill${on ? " on" : ""}`} aria-pressed={on}
      title={on ? "Searching by meaning: click to search by words" : "Search by meaning: a role or the work, in your words (talent acquisition, someone who hired engineers)"}
      onClick={() => { onChange(!on); }}>
      <span aria-hidden="true">≈</span> Meaning
    </button>
  );
}

/** How long a search may be read before the page says it is taking long. */
export const READING_TOO_LONG_MS = 15000;

/** How often to ask again while the words are read: soon at first, then
 * seldom (after a while the AI worker is likely down, and the page may stay open). */
export function readingInterval(reading: boolean, asked: number): number | false {
  if (!reading) return false;
  return asked > 20 ? 5000 : 700;
}

/** True once the search has been reading for longer than usual. */
export function useReadingTooLong(reading: boolean, text: string): boolean {
  const [longFor, setLongFor] = useState<string | null>(null); // the search that took long
  useEffect(() => {
    if (!reading) return undefined;
    const t = window.setTimeout(() => { setLongFor(text); }, READING_TOO_LONG_MS);
    return () => { window.clearTimeout(t); };
  }, [reading, text]);
  return reading && longFor === text;
}

/** What the search by meaning is doing, in a line above the list. */
export function MeaningBanner({ outcome, text, tooLong, onWords }: {
  outcome: Schemas["MeaningOutcome"] | null | undefined;
  text: string;
  tooLong: boolean;
  onWords: () => void;
}) {
  if (!outcome) return null;
  if (outcome.status === "unavailable") {
    return (
      <p className="meaning-banner warning" role="status">
        Search by meaning is not set up on this server (the AI worker has no meaning model).{" "}
        <button type="button" className="link" onClick={onWords}>Search by words instead</button>
      </p>
    );
  }
  if (outcome.status === "reading") {
    return (
      <p className="meaning-banner" role="status">
        <span className="spinner" aria-hidden="true" /> Reading “{text}” the way a recruiter would…
        {tooLong && <> This is taking longer than usual: is the AI worker running? <button type="button" className="link" onClick={onWords}>Search by words instead</button></>}
      </p>
    );
  }
  const u = outcome.understood;
  const read = outcome.read ?? 0;
  const total = outcome.total ?? 0;
  return (
    <p className="meaning-banner" role="status">
      {u?.describes ? (
        <>People whose work matches “{text}”{u.family && <> ({u.family})</>}.</>
      ) : (
        <>
          Searching for <strong>{u?.role ?? text}</strong>
          {u?.family && <> ({u.family})</>}
          {u?.levelName && <>, {u.levelName.toLowerCase()} level first</>}
          {u?.context && <> · {u.context}</>}
          , whatever their titles say.
        </>
      )}{" "}
      Best matches first.
      {total > 0 && read < total && <span className="muted"> Still reading CVs: {read.toLocaleString()} of {total.toLocaleString()} candidates so far.</span>}
    </p>
  );
}

const STRENGTH: Record<Schemas["MeaningWhy"]["strength"], string> = { strong: "Strong match", good: "Good match", possible: "Possible match" };

/** Why a candidate was found by meaning: the title of theirs that matched (its
 * field, and its level when the search asked for one), where their CV says it,
 * and the search's own words in it (unless shown otherwise). */
export function WhyLine({ why, showLevel = false, showWords = true }: { why: Schemas["MeaningWhy"]; showLevel?: boolean; showWords?: boolean }) {
  return (
    <span className="why-line">
      <span className={`why-strength ${why.strength}`}>{STRENGTH[why.strength]}</span>
      {why.title && (
        <span className="why-title">
          as <strong>{why.title}</strong> ({why.titleWhen === "now" || !why.titleWhen ? "now" : why.titleWhen})
          {why.family && <span className="muted"> · {why.family}</span>}
          {showLevel && why.level && <span className="muted"> · level: {why.level.toLowerCase()}</span>}
        </span>
      )}
      {why.passage && <q className="why-passage">{why.passage}</q>}
      {showWords && why.words.length > 0 && <span className="muted small why-words">CV says: {why.words.join(", ")}</span>}
    </span>
  );
}

/** Whether a typed search reads as a role or the work (worth searching by meaning), not a name, email or phone. */
export function looksLikeWork(text: string): boolean {
  const t = text.trim();
  return t.length >= 3 && !/[@\d]/.test(t) && /[a-z]{3,}/i.test(t);
}
