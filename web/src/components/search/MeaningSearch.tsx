// Search by meaning on the candidates page: the switch beside the search box,
// how the search was read (and how far the reading of the company's CVs has
// got), why each candidate was found, and the offer to search words that
// found no one by meaning. The server reads the words in a moment; meanwhile
// the list asks again (readingInterval).
import { useQuery } from "@tanstack/react-query";
import { Fragment, useEffect, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";

const KEY = "hiringplug.candidates.meaning";

/**
 * Whether the search box searches by meaning (kept on this device: a
 * convenience). byWords: the page opened with a search of its own in the
 * address (a name, email or phone from the dashboard), so this visit searches
 * by words until the switch is used; what is kept stays as it was.
 */
export function useMeaningMode(byWords = false): [boolean, (on: boolean) => void] {
  const [on, setOn] = useState(() => {
    if (byWords) return false;
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
  if (!reading || asked > READING_POLLS) return false; // a few minutes: the AI worker is likely down
  return asked > 20 ? 5000 : 700;
}

/** How often a search being read is asked about before the page stops asking (about 3½ minutes). */
export const READING_POLLS = 60;

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
export function MeaningBanner({ outcome, text, tooLong, onWords, onAgain }: {
  outcome: Schemas["MeaningOutcome"] | null | undefined;
  text: string;
  tooLong: boolean;
  onWords: () => void;
  onAgain?: () => void; // ask again, once the page has stopped asking
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
        {tooLong && <> This is taking longer than usual: is the AI worker running?{" "}
          {onAgain && <><button type="button" className="link" onClick={onAgain}>Check again</button> · </>}
          <button type="button" className="link" onClick={onWords}>Search by words instead</button></>}
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

/** How strong a match by meaning is, as a small label ("Strong match"). */
export function WhyStrength({ why }: { why: Schemas["MeaningWhy"] }) {
  return <span className={`why-strength ${why.strength}`}>{STRENGTH[why.strength]}</span>;
}

/** The matched title's field, and its level when the search asked for one ("accounting · level: executive"). */
function WhyField({ why, showLevel, lead }: { why: Schemas["MeaningWhy"]; showLevel: boolean; lead: string }) {
  const parts = [why.family, showLevel && why.level ? `level: ${why.level.toLowerCase()}` : null].filter(Boolean);
  if (parts.length === 0) return null;
  return <span className="muted why-field">{lead}{parts.join(" · ")}</span>;
}

/** Why a candidate was found by meaning: the title of theirs that matched (its
 * field, and its level when the search asked for one), where their CV says it,
 * and the search's own words in it (unless shown otherwise; each word whole,
 * the line breaking after a comma). inCell stacks it in a list's Match column,
 * the CV's words cut to three lines (all of them on hover). */
export function WhyLine({ why, showLevel = false, showWords = true, inCell = false }: {
  why: Schemas["MeaningWhy"]; showLevel?: boolean; showWords?: boolean; inCell?: boolean;
}) {
  return (
    <span className={`why-line${inCell ? " in-cell" : ""}`}>
      <WhyStrength why={why} />
      {why.title && (
        <span className="why-title">
          as <strong>{why.title}</strong> ({why.titleWhen === "now" || !why.titleWhen ? "now" : why.titleWhen})
          {!inCell && <WhyField why={why} showLevel={showLevel} lead=" · " />}
        </span>
      )}
      {why.title && inCell && <WhyField why={why} showLevel={showLevel} lead="" />}
      {why.passage && <q className="why-passage" title={inCell ? why.passage : undefined}>{why.passage}</q>}
      {showWords && why.words.length > 0 && (
        <span className="muted small why-words">
          CV says: {why.words.map((w, i) => <Fragment key={i}>{i > 0 && ", "}<span className="why-word">{w}</span></Fragment>)}
        </span>
      )}
    </span>
  );
}

/**
 * Whether a typed search could be a role or the work: not an email or a phone,
 * nor too short to mean anything. A name passes (names and roles cannot be
 * told apart), so the offer to search by meaning waits for words that found no one.
 */
export function looksLikeWork(text: string): boolean {
  const t = text.trim();
  return t.length >= 3 && !/[@\d]/.test(t) && /[a-z]{3,}/i.test(t);
}

/** Whether this server can search by meaning changes only with the AI worker's model: asked again after an hour. */
const STATUS_FRESH_MS = 60 * 60_000;

/**
 * Whether to offer searching a word search's text by meaning: the words found
 * no one, could be a role or the work, and this server has the meaning model
 * (asked once, and only when the offer could show).
 */
export function useMeaningOffer(text: string, nobodyFound: boolean): boolean {
  const ask = nobodyFound && looksLikeWork(text);
  const status = useQuery({
    queryKey: ["meaning", "status"],
    queryFn: async () => unwrap(await api.GET("/v1/meaning/status")),
    enabled: ask,
    staleTime: STATUS_FRESH_MS,
  });
  return ask && status.data?.available === true;
}

/** Under the search box while searching by meaning, when the words look like a boolean: its rules are not read. */
export function MeaningRulesHint({ id }: { id: string }) {
  return (
    <p id={id} className="meaning-hint muted small" role="status">
      Search by meaning reads your words as a recruiter would; AND, OR, NOT and quotes work in the word search.
    </p>
  );
}
