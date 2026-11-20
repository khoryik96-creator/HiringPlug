// A message being written in Outlook Mail: its own part (above the quoted
// message of a reply or forward, which starts at a rule or a quote), and the
// person's signature, added once, between the two, as Outlook does.

const SIGNATURE = "[data-signature]";

function parse(html: string): HTMLElement {
  return new DOMParser().parseFromString(`<body><div>${html}</div></body>`, "text/html").body.firstElementChild as HTMLElement;
}

/** The node where the quoted message starts (a rule or a quote at the top level), if any. */
function quoteStart(root: HTMLElement): Element | null {
  for (const node of Array.from(root.children)) {
    if (node.matches("hr, blockquote") || node.querySelector(":scope > hr")) return node;
  }
  return null;
}

export const hasSignature = (html: string) => html.includes("data-signature");

/** The message with the signature put in: before the quoted message, or at the end; with a line above it to type on. */
export function withSignature(html: string, signature: string): string {
  if (!signature.trim() || hasSignature(html)) return html;
  const root = parse(html);
  const block = root.ownerDocument.createElement("div");
  block.setAttribute("data-signature", "true");
  block.innerHTML = signature;
  // A signature saved inside its own marker is not marked twice.
  const inner = block.querySelector(SIGNATURE);
  if (inner && block.children.length === 1) block.innerHTML = inner.innerHTML;
  const quote = quoteStart(root);
  root.insertBefore(block, quote);
  // Room above it to write in.
  const before = block.previousElementSibling;
  if (!before || before.textContent.trim() !== "" || before.matches(SIGNATURE)) {
    const line = root.ownerDocument.createElement("p");
    line.innerHTML = "<br>";
    root.insertBefore(line, block);
  }
  if (!root.firstChild || root.firstChild === block) root.insertBefore(Object.assign(root.ownerDocument.createElement("p"), { innerHTML: "<br>" }), block);
  return root.innerHTML;
}

/** What the person wrote: above the quoted message, without the signature (for a template). */
export function ownPart(html: string): string {
  const root = parse(html);
  const quote = quoteStart(root);
  if (quote) {
    while (quote.nextSibling) quote.nextSibling.remove();
    quote.remove();
  }
  root.querySelectorAll(SIGNATURE).forEach((s) => { s.remove(); });
  return root.innerHTML.trim();
}

/** The message with html put at its start (a template in a reply goes above the quoted message). */
export function atStart(html: string, insert: string): string {
  const root = parse(html);
  const first = root.firstElementChild;
  // An empty first line (where the caret starts) is replaced.
  if (first && first.matches("p, div") && first.textContent.trim() === "" && !first.querySelector("img") && !first.matches(SIGNATURE)) first.remove();
  const holder = parse(insert);
  root.prepend(...Array.from(holder.childNodes));
  return root.innerHTML;
}
