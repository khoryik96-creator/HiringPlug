// Small helpers for the email editor (a contenteditable box): commands from
// the toolbar, inserting at the cursor, and plain text as HTML.

export function escapeHtml(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

/** A plain-text template as paragraphs, keeping its line breaks. */
export function plainToHtml(text: string): string {
  return text.split(/\n{2,}/).map((p) => `<p>${escapeHtml(p).replace(/\n/g, "<br>")}</p>`).join("");
}

/** Runs a formatting command on the selection; false when the browser has none. */
export function exec(command: string, value?: string): boolean {
  // Browsers offer nothing else for formatting editable text (every rich
  // email editor uses it); it may be missing, as in tests.
  const doc = document as Document & { execCommand?: (c: string, ui: boolean, v?: string) => boolean };
  // eslint-disable-next-line @typescript-eslint/no-deprecated
  if (typeof doc.execCommand !== "function") return false;
  // eslint-disable-next-line @typescript-eslint/no-deprecated
  return doc.execCommand(command, false, value);
}

/** Where the cursor is in the editor, to put it back after a toolbar dropdown took the focus. */
export function saveRange(editor: HTMLElement): Range | null {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  return editor.contains(range.commonAncestorContainer) ? range.cloneRange() : null;
}

/** Focuses the editor with the cursor back where it was, or at its end. */
export function restoreRange(editor: HTMLElement, range: Range | null): void {
  editor.focus();
  const sel = window.getSelection();
  if (!sel) return;
  let at = range;
  if (!at || !editor.contains(at.commonAncestorContainer)) {
    at = document.createRange();
    at.selectNodeContents(editor);
    at.collapse(false);
  }
  sel.removeAllRanges();
  sel.addRange(at);
}

/** Inserts HTML at the cursor in the editor, or at its end when the cursor is elsewhere. */
export function insertHtml(editor: HTMLElement, html: string): void {
  const sel = window.getSelection();
  const range = sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
  if (!range || !editor.contains(range.commonAncestorContainer)) {
    editor.insertAdjacentHTML("beforeend", html);
    return;
  }
  if (exec("insertHTML", html)) return;
  range.deleteContents();
  const fragment = range.createContextualFragment(html);
  const last = fragment.lastChild;
  range.insertNode(fragment);
  if (last) {
    range.setStartAfter(last);
    range.collapse(true);
    sel?.removeAllRanges();
    sel?.addRange(range);
  }
}

/** Point font size 7 (what the fontSize command makes) at a size in points. */
export function setFontSize(editor: HTMLElement, pt: string): void {
  if (!exec("fontSize", "7")) return;
  editor.querySelectorAll('font[size="7"]').forEach((f) => {
    const span = document.createElement("span");
    span.style.fontSize = `${pt}pt`;
    span.innerHTML = f.innerHTML;
    f.replaceWith(span);
  });
}

/** Shows banner images in the editor or a preview (their address is a local blob). */
export function showBanners(root: HTMLElement | null, urls: Record<string, string>): void {
  root?.querySelectorAll<HTMLImageElement>("img[data-banner-id]").forEach((img) => {
    const url = urls[img.dataset.bannerId ?? ""];
    if (url && img.getAttribute("src") !== url) img.setAttribute("src", url);
    img.style.maxWidth = "100%";
  });
}

/**
 * An invisible character after a value, so the cursor is outside it and what
 * is typed next is not part of the value. Taken out before the message is
 * previewed, sent or saved (withoutCaretMarks).
 */
const CARET = "\u200b";

export function withoutCaretMarks(html: string): string {
  return html.replaceAll(CARET, "");
}

/** Puts a field's value at the cursor (or the end), the cursor after it. */
export function insertValue(editor: HTMLElement, key: string, value: string): void {
  const span = document.createElement("span");
  span.dataset.field = key;
  span.textContent = value;
  const after = document.createTextNode(CARET);
  const sel = window.getSelection();
  let range = sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
  if (!range || !editor.contains(range.commonAncestorContainer)) {
    range = document.createRange();
    const last = editor.lastElementChild;
    range.selectNodeContents(last && /^(P|DIV)$/.test(last.tagName) ? last : editor);
    range.collapse(false);
  }
  range.deleteContents();
  range.insertNode(after);
  range.insertNode(span);
  range.setStart(after, 1);
  range.collapse(true);
  sel?.removeAllRanges();
  sel?.addRange(range);
}

/**
 * Puts each typed field that has a value ("[Expected salary]") as its value,
 * the cursor after it when it was just typed. False when there was none.
 */
export function fillTypedFields(editor: HTMLElement, fields: { token: string; key: string }[], values: Record<string, string>): boolean {
  const known = fields.filter((f) => values[f.token]);
  if (known.length === 0) return false;
  const pattern = new RegExp(known.map((f) => f.token.replace(/[[\]]/g, "\\$&")).join("|"), "gi");
  const walker = document.createTreeWalker(editor, NodeFilter.SHOW_TEXT, {
    acceptNode: (n) => (n.parentElement?.closest("[data-field]") ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
  });
  const texts: Text[] = [];
  for (let n = walker.nextNode(); n; n = walker.nextNode()) texts.push(n as Text);
  const sel = window.getSelection();
  let changed = false;
  for (const text of texts) {
    const matches = [...text.data.matchAll(pattern)];
    if (matches.length === 0) continue;
    const hadCursor = sel?.anchorNode === text;
    const parts = document.createDocumentFragment();
    let at = 0;
    for (const m of matches) {
      const field = known.find((f) => f.token.toLowerCase() === m[0].toLowerCase());
      if (!field) continue;
      parts.append(text.data.slice(at, m.index));
      const span = document.createElement("span");
      span.dataset.field = field.key;
      span.textContent = values[field.token] ?? "";
      parts.append(span);
      at = m.index + m[0].length;
    }
    const rest = document.createTextNode(CARET + text.data.slice(at));
    parts.append(rest);
    text.replaceWith(parts);
    changed = true;
    if (hadCursor) {
      const range = document.createRange();
      range.setStart(rest, 1);
      range.collapse(true);
      sel.removeAllRanges();
      sel.addRange(range);
    }
  }
  return changed;
}

/** Fields in plain text (a subject) as their values; fields without one stay as [Field]. */
export function fillTokens(text: string, fields: { token: string }[], values: Record<string, string>): string {
  const known = fields.filter((f) => values[f.token]);
  if (known.length === 0) return text;
  const pattern = new RegExp(known.map((f) => f.token.replace(/[[\]]/g, "\\$&")).join("|"), "gi");
  return text.replace(pattern, (m) => values[known.find((f) => f.token.toLowerCase() === m.toLowerCase())?.token ?? ""] ?? m);
}

/** Puts text into an input's value where its cursor is (at the end when it never had one). */
export function atCursor(input: HTMLInputElement | null, current: string, text: string): string {
  const at = input?.selectionStart ?? current.length;
  const end = input?.selectionEnd ?? at;
  return current.slice(0, at) + text + current.slice(end);
}

/**
 * A subject filled in for one candidate back as fields ("Siti - RM 12,000" ->
 * "[First name] - [Expected salary]"), so a template saved from an email never
 * keeps their details. Longer values first, so "Siti Aisyah" is [Full name].
 */
export function valuesToTokens(text: string, values: Record<string, string>): string {
  const known = Object.entries(values).filter(([, v]) => v.trim().length > 1).sort(([, a], [, b]) => b.length - a.length);
  let out = text;
  for (const [token, value] of known) out = out.split(value).join(token);
  return out;
}
