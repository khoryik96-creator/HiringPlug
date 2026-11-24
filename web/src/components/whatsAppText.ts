// Plain-text helpers for WhatsApp messages: putting text in at the cursor of a
// message box, and turning one person's details back into template fields.
import { type RefObject, useLayoutEffect, useRef } from "react";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/**
 * The fields a message's values may be turned back into, first winning where
 * two share a value: who the message is to and from, and the job. A place, an
 * industry or a notice period ("Kuala Lumpur", "Finance") is as likely to be
 * about the job as about the person, and their current title or company is
 * often the job's own, so those stay as written.
 */
const reversible = ["job.title", "company.name", "candidate.fullName", "candidate.firstName", "candidate.lastName", "client.name", "sender.name", "agency.name"];

/**
 * A message written for one person back as a template ("Hi Siti Aminah" ->
 * "Hi {{candidate.fullName}}"), so it works for anyone. Longer values first
 * (a full name before a first name), whole words only ("Ali" inside "Alif"
 * stays), and values shorter than two characters are left alone. A field
 * that had no value ("[job.title]" in the message) becomes a field again, and
 * fields already written as {{...}} stay as they are.
 */
export function valuesToFields(text: string, fields: { key: string; value: string }[]): string {
  const byValue = new Map<string, string>();
  for (const key of reversible) {
    const value = fields.find((f) => f.key === key)?.value ?? "";
    if (value.trim().length >= 2 && !byValue.has(value)) byValue.set(value, key);
  }
  const keys = fields.map((f) => f.key);
  if (byValue.size === 0 && keys.length === 0) return text;
  const values = [...byValue.keys()].sort((a, b) => b.length - a.length);
  const parts = ["\\{\\{[^{}]*\\}\\}"];
  if (keys.length > 0) parts.push(`\\[(${keys.map(escape).join("|")})\\]`);
  if (values.length > 0) parts.push(`(?<![\\p{L}\\p{N}_])(?:${values.map(escape).join("|")})(?![\\p{L}\\p{N}_])`);
  // One pass, so a field already put in is never matched again.
  return text.replace(new RegExp(parts.join("|"), "gu"), (found, bracketed: unknown) => {
    if (found.startsWith("{{")) return found;
    return `{{${typeof bracketed === "string" ? bracketed : byValue.get(found) ?? found}}}`;
  });
}

/**
 * Puts text in a message box where its cursor is (over any selected text), then
 * leaves the cursor just after it with the box still focused. A box that has
 * not been in focus yet has no cursor of the person's own (browsers report the
 * start), so the text goes at the end, as it does without a box.
 */
export function useInsertAtCursor(box: RefObject<HTMLTextAreaElement | null>, current: string, onChange: (next: string) => void) {
  const caret = useRef<number | null>(null);
  // The boxes that have been in focus: a box shown again is a new one.
  const focused = useRef(new WeakSet<HTMLTextAreaElement>());
  useLayoutEffect(() => {
    const node = box.current;
    if (!node) return;
    const mark = () => { focused.current.add(node); };
    if (document.activeElement === node) mark();
    node.addEventListener("focus", mark);
    return () => { node.removeEventListener("focus", mark); };
  });
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
    const cursor = node && focused.current.has(node) ? node : null;
    const start = Math.min(cursor?.selectionStart ?? current.length, current.length);
    const end = Math.max(start, Math.min(cursor?.selectionEnd ?? start, current.length));
    caret.current = start + text.length;
    onChange(current.slice(0, start) + text + current.slice(end));
  };
}
