/**
 * An ad's text as candidates will read it: paragraphs, and bullet points for
 * lines starting with "- " or "• ". Plain text only (never HTML), so nothing
 * typed into an ad can run in a candidate's browser.
 */
export function AdText({ text }: { text: string }) {
  const blocks: { bullets: boolean; lines: string[] }[] = [];
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) {
      blocks.push({ bullets: false, lines: [] });
      continue;
    }
    const bullet = /^[-•*]\s+/.exec(line);
    const last = blocks.at(-1);
    const content = bullet ? line.slice(bullet[0].length) : line;
    if (last && last.bullets === Boolean(bullet) && last.lines.length > 0) last.lines.push(content);
    else blocks.push({ bullets: Boolean(bullet), lines: [content] });
  }
  return (
    <div className="ad-text">
      {blocks.filter((b) => b.lines.length > 0).map((b, i) =>
        b.bullets ? (
          <ul key={i}>{b.lines.map((l, j) => <li key={j}>{l}</li>)}</ul>
        ) : (
          <p key={i}>{b.lines.map((l, j) => <span key={j}>{j > 0 && <br />}{l}</span>)}</p>
        ),
      )}
    </div>
  );
}
