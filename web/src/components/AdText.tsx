import type { ReactNode } from "react";

/**
 * Text as people read it: **bold** words, and a line that is only bold (or
 * starts with #) as a heading, as the AI writes them. Plain text otherwise:
 * nothing becomes HTML.
 */
export function Inline({ text }: { text: string }): ReactNode {
  const parts = text.split(/(\*\*[^*\n]+\*\*)/g);
  return parts.map((part, i) => (/^\*\*[^*\n]+\*\*$/.test(part) ? <strong key={i}>{part.slice(2, -2)}</strong> : part));
}

/** A line that is a heading: "## Pay" or "**Pay**" on its own. */
function headingOf(line: string): string | null {
  const hash = /^#{1,6}\s+(.+)$/.exec(line);
  if (hash?.[1]) return hash[1].replace(/^\*\*(.+)\*\*$/, "$1");
  const bold = /^\*\*([^*]+)\*\*:?$/.exec(line);
  return bold?.[1] ?? null;
}

/**
 * An ad's text as candidates will read it (and the AI's answers): paragraphs,
 * headings, and bullet points for lines starting with "- ", "• " or "* ".
 * Plain text only (never HTML), so nothing typed into an ad can run in a
 * candidate's browser.
 */
export function AdText({ text }: { text: string }) {
  const blocks: { bullets: boolean; heading?: boolean; lines: string[] }[] = [];
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) {
      blocks.push({ bullets: false, lines: [] });
      continue;
    }
    const heading = headingOf(line);
    if (heading) {
      blocks.push({ bullets: false, heading: true, lines: [heading] });
      continue;
    }
    const bullet = /^[-•*]\s+/.exec(line);
    const last = blocks.at(-1);
    const content = bullet ? line.slice(bullet[0].length) : line;
    if (last && !last.heading && last.bullets === Boolean(bullet) && last.lines.length > 0) last.lines.push(content);
    else blocks.push({ bullets: Boolean(bullet), lines: [content] });
  }
  return (
    <div className="ad-text">
      {blocks.filter((b) => b.lines.length > 0).map((b, i) =>
        b.heading ? (
          <p key={i} className="ad-heading"><strong>{b.lines[0]}</strong></p>
        ) : b.bullets ? (
          <ul key={i}>{b.lines.map((l, j) => <li key={j}><Inline text={l} /></li>)}</ul>
        ) : (
          <p key={i}>{b.lines.map((l, j) => <span key={j}>{j > 0 && <br />}<Inline text={l} /></span>)}</p>
        ),
      )}
    </div>
  );
}
