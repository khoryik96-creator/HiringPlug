// Plain-text helpers for WhatsApp messages: putting text in at the cursor of a
// message box, and turning one person's details back into template fields.
import { type RefObject, useLayoutEffect, useRef, useState } from "react";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
/** A value as a whole word: "Ali" matches in "Ali's" but not in "Alif". */
const whole = (pattern: string) => `(?<![\\p{L}\\p{N}_])(?:${pattern})(?![\\p{L}\\p{N}_])`;
const written = "\\{\\{[^{}]*\\}\\}";

/**
 * The fields a message's values may be turned back into, first winning where
 * two share a value: who the message is to and from, and the job. A place, an
 * industry or a notice period ("Kuala Lumpur", "Finance") is as likely to be
 * about the job as about the person, and their current title or company is
 * often the job's own, so those stay as written (unless the template the
 * message came from used them).
 */
const reversible = ["job.title", "company.name", "candidate.fullName", "candidate.firstName", "candidate.lastName", "client.name", "sender.name", "agency.name"];

type Field = { key: string; value: string };

/**
 * Whether a single-word value ("May", "Will", "Hope") found at `at` stands
 * where an ordinary capitalised word would: at the start of a sentence or a
 * line, not followed by a comma ("May, are you free?") and not the only word
 * of its line (a name signed at the end).
 */
function looksLikeAWord(text: string, at: number, found: string): boolean {
  if (/\s/u.test(found)) return false;
  const before = text.slice(0, at);
  if (!/(?:^|[.!?]\s|\n)\s*$/u.test(before)) return false;
  const after = text.slice(at + found.length);
  if (after.startsWith(",")) return false;
  const alone = before.slice(before.lastIndexOf("\n") + 1).trim() === "" && /^[^\S\n]*[.!]?[^\S\n]*(?:\n|$)/u.test(after);
  return !alone;
}

/**
 * A message written for one person back as a template ("Hi Siti Aminah" ->
 * "Hi {{candidate.fullName}}"), so it works for anyone. Longer values first
 * (a full name before a first name), whole words only ("Ali" inside "Alif"
 * stays), and values shorter than two characters are left alone. A field
 * that had no value ("[job.title]" in the message) becomes a field again, and
 * fields already written as {{...}} stay as they are. `also` adds fields to
 * turn back, after the usual ones: those the source template used.
 */
export function valuesToFields(text: string, fields: Field[], also: string[] = []): string {
  const valueOf = (key: string) => fields.find((f) => f.key === key)?.value ?? "";
  const byValue = new Map<string, string>();
  for (const key of [...reversible, ...also]) {
    const value = valueOf(key);
    // Someone with no last name: their full name is their first name, and
    // "Hi Siti" means the first name. (The client contact's name, the same
    // as a contact's own, already comes after both.)
    if (key === "candidate.fullName" && value === valueOf("candidate.firstName")) continue;
    if (value.trim().length >= 2 && !byValue.has(value)) byValue.set(value, key);
  }
  const keys = fields.map((f) => f.key);
  if (byValue.size === 0 && keys.length === 0) return text;
  const values = [...byValue.keys()].sort((a, b) => b.length - a.length);
  const parts = [written];
  if (keys.length > 0) parts.push(`\\[(${keys.map(escape).join("|")})\\]`);
  if (values.length > 0) parts.push(whole(values.map(escape).join("|")));
  // One pass, so a field already put in is never matched again.
  let out = "";
  let last = 0;
  for (const m of text.matchAll(new RegExp(parts.join("|"), "gu"))) {
    const [found, bracketed] = m;
    const key = found.startsWith("{{") ? undefined : bracketed ?? byValue.get(found);
    if (key === undefined || (bracketed === undefined && looksLikeAWord(text, m.index, found))) continue;
    out += `${text.slice(last, m.index)}{{${key}}}`;
    last = m.index + found.length;
  }
  return out + text.slice(last);
}

/** The fields a template's text uses ("{{job.title}}" -> "job.title"). */
export const templateKeys = (body: string) => [...body.matchAll(/\{\{\s*([^{}\s]+)\s*\}\}/gu)].map((m) => m[1] ?? "");

/** The fields still written "[key]" (no value) in a message, in the order they appear. */
export function unfilledKeys(text: string, keys: string[]): string[] {
  if (keys.length === 0) return [];
  const found = [...text.matchAll(new RegExp(`\\[(${keys.map(escape).join("|")})\\]`, "gu"))].map((m) => m[1] ?? "");
  return [...new Set(found)];
}

/** The fields whose value (two characters or more) is still in a template's text, once per value, longer first. */
export function valuesLeft<F extends Field>(text: string, fields: F[]): F[] {
  const plain = text.replace(new RegExp(written, "gu"), " ");
  const left = new Map<string, F>();
  // A value left as an ordinary word ("May I call you?") is not pointed out.
  const asValue = (value: string) => [...plain.matchAll(new RegExp(whole(escape(value)), "gu"))].some((m) => !looksLikeAWord(plain, m.index, m[0]));
  for (const f of fields) {
    if (f.value.trim().length >= 2 && !left.has(f.value) && asValue(f.value)) left.set(f.value, f);
  }
  return [...left.values()].sort((a, b) => b.value.length - a.value.length);
}

/**
 * Every whole-word `value` in a template's text (outside fields) becomes
 * {{key}}, except where it starts a sentence as an ordinary word ("May I").
 */
export const valueToField = (text: string, value: string, key: string) =>
  text.replace(new RegExp(`${written}|${whole(escape(value))}`, "gu"), (found: string, ...rest: unknown[]) => {
    const at = rest.find((r): r is number => typeof r === "number") ?? 0;
    return found.startsWith("{{") || looksLikeAWord(text, at, found) ? found : `{{${key}}}`;
  });

/**
 * Puts text in a message box where its cursor is (over any selected text), then
 * leaves the cursor just after it with the box still focused. A box that has
 * not been in focus yet has no cursor of the person's own (browsers report the
 * start), so the text goes at the end, as it does without a box.
 */
export function useInsertAtCursor(box: RefObject<HTMLTextAreaElement | null>, current: string, onChange: (next: string) => void) {
  const caret = useRef<number | null>(null);
  // The boxes that have been in focus: a box shown again is a new one.
  const [focused] = useState(() => new WeakSet<HTMLTextAreaElement>());
  // One listener for as long as the hook is used, whichever box is shown.
  useLayoutEffect(() => {
    const mark = (e: FocusEvent) => { if (box.current && e.target === box.current) focused.add(box.current); };
    document.addEventListener("focusin", mark);
    return () => { document.removeEventListener("focusin", mark); };
  }, [box, focused]);
  useLayoutEffect(() => {
    const node = box.current;
    const at = caret.current;
    if (!node || at === null) return;
    caret.current = null;
    node.focus();
    node.setSelectionRange(at, at);
  });
  return (text: string) => {
    const node = box.current;
    const cursor = node && (focused.has(node) || document.activeElement === node) ? node : null;
    const start = Math.min(cursor?.selectionStart ?? current.length, current.length);
    const end = Math.max(start, Math.min(cursor?.selectionEnd ?? start, current.length));
    caret.current = start + text.length;
    onChange(current.slice(0, start) + text + current.slice(end));
  };
}
