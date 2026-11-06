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

/** Inserts HTML at the cursor in the editor, or at its end when the cursor is elsewhere. */
export function insertHtml(editor: HTMLElement, html: string): void {
  const sel = window.getSelection();
  const inside = sel && sel.rangeCount > 0 && editor.contains(sel.getRangeAt(0).commonAncestorContainer);
  if (inside && exec("insertHTML", html)) return;
  editor.insertAdjacentHTML("beforeend", html);
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
