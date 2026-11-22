// A small CSV reader for files saved by Excel or Google Sheets: quoted fields
// (with commas, quotes and line breaks inside), a byte-order mark, and the
// semicolon separator Excel uses in some countries.

/** Parses CSV text into rows of cells. Empty trailing lines are dropped. */
export function parseCsv(text: string): string[][] {
  const src = text.charCodeAt(0) === 0xfeff ? text.slice(1) : text; // byte-order mark
  const firstLine = src.split(/\r?\n/, 1)[0] ?? "";
  const sep = (firstLine.match(/;/g)?.length ?? 0) > (firstLine.match(/,/g)?.length ?? 0) ? ";" : ",";
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  for (let i = 0; i < src.length; i++) {
    const ch = src.charAt(i);
    if (quoted) {
      if (ch === '"' && src.charAt(i + 1) === '"') { cell += '"'; i++; }
      else if (ch === '"') quoted = false;
      else cell += ch;
    } else if (ch === '"' && cell === "") quoted = true;
    else if (ch === sep) { row.push(cell); cell = ""; }
    else if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && src.charAt(i + 1) === "\n") i++;
      row.push(cell);
      rows.push(row);
      row = [];
      cell = "";
    } else cell += ch;
  }
  if (cell !== "" || row.length > 0) { row.push(cell); rows.push(row); }
  return rows.filter((r) => r.some((c) => c.trim() !== ""));
}

/** Writes rows as CSV (quoting where needed), e.g. the rows to fix. */
export function toCsv(rows: string[][]): string {
  const quote = (c: string) => (/[",\n\r]/.test(c) ? `"${c.replace(/"/g, '""')}"` : c);
  return rows.map((r) => r.map(quote).join(",")).join("\r\n") + "\r\n";
}

/**
 * The rows to fix, with a Problem column: each row padded to the heading
 * (a row from Excel stops at its last filled cell), so its problem sits
 * under Problem. line is the row's line in the file (2 = the first row).
 */
export function rowsToFix(header: string[], rows: string[][], problems: { line: number; message: string }[]): string[][] {
  const padded = (cells: string[]) => [...cells, ...Array<string>(Math.max(0, header.length - cells.length)).fill("")];
  return [[...header, "Problem"], ...problems.map((p) => [...padded(rows[p.line - 2] ?? []), p.message])];
}

/** The rows-to-fix file's name: always a CSV ("Candidates.xlsx" gives "to-fix-Candidates.csv"). */
export function toFixName(fileName: string): string {
  return `to-fix-${fileName.replace(/\.[^.]+$/, "")}.csv`;
}
