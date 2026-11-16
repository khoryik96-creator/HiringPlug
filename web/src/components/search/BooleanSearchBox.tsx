// The search box of the candidate, company and contact lists. Typed plainly,
// it finds names, emails, phones and the like; as soon as it holds boolean
// rules (AND, OR, NOT, NEAR/n, brackets, "phrases", word*, -word) those show
// in blue and the list runs a boolean search instead.
import { type ReactNode, useId, useLayoutEffect, useRef } from "react";

import { ErrorMessage } from "../ErrorMessage";
import { fieldErrors } from "../forms";
import { BOOL_OPERATOR, BOOL_TOKEN } from "./candidateSearch";

/**
 * Whether a search holds boolean rules: operators in capitals (so "Johnson
 * and Johnson" stays a name search), "quotes", word*, -word or !word.
 * Brackets alone do not count: "(03) 2345 6789" is a phone number.
 */
export function looksBoolean(q: string): boolean {
  // An operator between two terms ("AND Digital" or "NEAR Protocol" alone is a name).
  return /\S\s+(AND|OR|NOT|NEAR(\/\d+)?|W\/\d+|&&|\|\|)\s+\S/.test(q)
    || /["“”]/.test(q)
    || /(^|[\s(])[-!][^\s\d-]/.test(q) // -intern, !sales (not "Tan - Maju" or -5)
    || /[^\s*]\*(\s|$|\))/.test(q); // manag*
}

export type BoolPart = { text: string; rule: boolean };

/** A search split into its boolean rules (shown in blue) and the words looked for, as candidateSearch's booleanTokens splits it. */
export function booleanParts(q: string): BoolPart[] {
  const out: BoolPart[] = [];
  const re = new RegExp(BOOL_TOKEN.source, "g");
  let m: RegExpExecArray | null;
  while ((m = re.exec(q)) !== null) {
    const [tok, space, phrase, group, word] = m;
    if (space) { out.push({ text: tok, rule: false }); continue; }
    if (group || phrase) { out.push({ text: tok, rule: true }); continue; }
    if (!word) { out.push({ text: tok, rule: false }); continue; } // a stray character, kept as typed
    if (BOOL_OPERATOR.test(word)) { out.push({ text: word, rule: true }); continue; } // as the server reads it, in any case
    // -word or !word: the sign is the rule; word*: the star is.
    const sign = /^[-!+](?=\S)/.exec(word)?.[0] ?? "";
    const star = word.length > sign.length + 1 && word.endsWith("*") ? "*" : "";
    if (sign) out.push({ text: sign, rule: true });
    out.push({ text: word.slice(sign.length, word.length - star.length), rule: false });
    if (star) out.push({ text: star, rule: true });
  }
  return out;
}

export function BooleanSearchBox({ value, onChange, placeholder, label, children }: {
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  label: string;
  /** Buttons inside the box, at its right (Mine, Filters, AI). */
  children?: ReactNode;
}) {
  const input = useRef<HTMLInputElement>(null);
  const mirror = useRef<HTMLDivElement>(null);
  const boolean = looksBoolean(value);
  const hintId = useId();
  // The coloured copy sits exactly under the typed text: same font, padding and scroll.
  useLayoutEffect(() => {
    const i = input.current;
    const m = mirror.current;
    if (!i || !m) return;
    const s = getComputedStyle(i);
    for (const p of ["font", "letterSpacing", "wordSpacing", "paddingLeft", "paddingRight", "paddingTop", "paddingBottom", "borderLeftWidth", "textIndent"] as const) {
      m.style[p] = s[p];
    }
    m.scrollLeft = i.scrollLeft;
  });
  return (
    <div className={`bool-box${boolean ? " is-boolean" : ""}`}>
      <svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
      <div className="bool-field">
        {boolean && (
          <div ref={mirror} className="bool-mirror" aria-hidden="true">
            {booleanParts(value).map((p, i) => <span key={i} className={p.rule ? "bool-rule" : undefined}>{p.text}</span>)}
          </div>
        )}
        <input ref={input} aria-label={label} placeholder={placeholder} value={value} spellCheck={!boolean}
          aria-describedby={boolean ? hintId : undefined}
          onChange={(e) => { onChange(e.target.value); }}
          onScroll={(e) => { if (mirror.current) mirror.current.scrollLeft = e.currentTarget.scrollLeft; }} />
      </div>
      {boolean && <span id={hintId} className="bool-badge" title="AND, OR, NOT, NEAR/n, ( ), &quot;phrase&quot;, word*, -word">Boolean</span>}
      {children}
    </div>
  );
}

/** A list's error: what is wrong with its boolean search, said plainly, or any other error. */
export function ListSearchError({ error }: { error: unknown }) {
  const problem = fieldErrors(error).boolean;
  if (problem) return <p className="bool-error" role="alert">This boolean search {problem}.</p>;
  return <ErrorMessage error={error} />;
}
