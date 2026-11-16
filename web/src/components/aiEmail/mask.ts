// What goes to the AI with an email, and how its answer comes back: names,
// email addresses, phone numbers, amounts, links and fields become
// placeholders (⟦1⟧, ⟦2⟧…; ⟦NAME⟧ the person it is to, ⟦ME⟧ the sender), and
// are put back, as they were, in what the AI writes. So the AI never sees
// pay or contact details, and the email keeps its fields and links.
import { escapeHtml } from "../clientEmail/richText";

export interface Names {
  /** The person it is to (their full name). */
  them?: string;
  /** The person sending. */
  me?: string;
  /** Other people the email names (candidates in a shortlist, those it goes to). */
  others?: string[];
}

/** What each ⟦n⟧ stands for, as HTML. */
export type Held = Map<string, string>;

const PATTERNS = [
  /[\w.+-]+@[\w-]+(?:\.[\w-]+)+/g, // email addresses
  /\bhttps?:\/\/[^\s<>"']+/gi, // links
  /(?:\b(?:RM|MYR|SGD|USD|IDR|S\$)|\$)\s?\d[\d,]*(?:\.\d+)?\s?k?\b/gi, // money
  /\+?\d[\d\s-]{7,}\d/g, // phone numbers
  /\b\d[\d,.]*\s?k\b|\b\d{1,3}(?:,\d{3})+(?:\.\d+)?\b|\b(?!(?:19|20)\d\d\b)\d{4,}\b/gi, // amounts (not years)
];

const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** Plain text with the private parts held back as placeholders. */
export function maskText(text: string, held: Held, names: Names = {}): string {
  let out = text;
  const hold = (raw: string, html = escapeHtml(raw)) => {
    for (const [k, v] of held) if (v === html) return k; // the same thing, the same placeholder
    const key = `⟦${String(held.size + 1)}⟧`;
    held.set(key, html);
    return key;
  };
  for (const re of PATTERNS) out = out.replace(re, (m) => hold(m));
  // A name comes back exactly as it was written: the whole name (in any
  // case) and each word of it where it starts with a capital, as names do
  // ("Will", not "will"; "Owner" of "Demo owner", not "the owner").
  // ⟦NAME⟧ is only the first name of the person it is to, ⟦ME⟧ only the
  // sender's whole name: the AI uses them for the greeting and sign-off.
  const swap = (text: string, flags: string, to: (m: string) => string) => {
    out = out.replace(new RegExp(`(?<![\\p{L}\\p{N}])${escapeRe(text)}(?![\\p{L}\\p{N}])`, `g${flags}u`), to);
  };
  const people: [string, "them" | "me" | "other"][] = [
    ...(names.them?.trim() ? [[names.them.trim(), "them"] as [string, "them"]] : []),
    ...(names.me?.trim() ? [[names.me.trim(), "me"] as [string, "me"]] : []),
    ...(names.others ?? []).filter((n) => n.trim()).map((n) => [n.trim(), "other"] as [string, "other"]),
  ];
  // Longest first, so a whole name goes before its words.
  people.sort((a, b) => b[0].length - a[0].length);
  for (const [name, who] of people) swap(name, "i", (m) => (who === "me" && m === name ? "⟦ME⟧" : hold(m)));
  const first = names.them?.trim().split(/\s+/)[0];
  for (const [name] of people) {
    for (const word of name.split(/\s+/).filter((w) => w.length > 1)) {
      swap(word, "i", (m) => (m === first ? "⟦NAME⟧" : m[0] === m[0]?.toLowerCase() ? m : hold(m)));
    }
  }
  return out;
}

/** The editor's HTML as plain text (paragraphs apart), private parts held back. */
export function maskHtml(html: string, held: Held, names: Names = {}): string {
  const doc = new DOMParser().parseFromString(`<div id="m">${html}</div>`, "text/html");
  const root = doc.getElementById("m");
  if (!root) return "";
  // Fields and links go back exactly as they were (their look, their target).
  const keep = (el: Element) => {
    const key = `⟦${String(held.size + 1)}⟧`;
    held.set(key, el.outerHTML);
    el.replaceWith(doc.createTextNode(key));
  };
  root.querySelectorAll("span[data-field]").forEach(keep);
  root.querySelectorAll("a[href]").forEach(keep);
  root.querySelectorAll("br").forEach((br) => { br.replaceWith(doc.createTextNode("\n")); });
  root.querySelectorAll("p, div, li, h1, h2, h3, h4, blockquote").forEach((el) => { el.append(doc.createTextNode("\n\n")); });
  const text = (root.textContent || "").replace(/\u00a0/g, " ").replace(/[ \t]+\n/g, "\n").replace(/\n{3,}/g, "\n\n").trim();
  return maskText(text, held, names);
}

/** What the AI wrote, as the editor's HTML, with every placeholder put back. */
export function unmask(text: string, held: Held, names: Names = {}): string {
  const first = names.them?.trim().split(/\s+/)[0] ?? "";
  const back = (s: string) => escapeHtml(s)
    .replace(/⟦(\d+)⟧/g, (m) => held.get(m) ?? m)
    .replace(/⟦NAME⟧/g, escapeHtml(first || "there"))
    .replace(/⟦ME⟧/g, escapeHtml(names.me?.trim() ?? ""));
  return text.trim().split(/\n{2,}/).map((p) => `<p>${back(p).replace(/\n/g, "<br>")}</p>`).join("");
}

/** A subject line with its placeholders put back, as text. */
export function unmaskSubject(text: string, held: Held, names: Names = {}): string {
  const div = document.createElement("div");
  div.innerHTML = unmask(text, held, names);
  return (div.textContent || "").trim();
}
