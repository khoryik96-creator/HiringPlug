// Reads the first sheet of an Excel workbook (.xlsx) into rows of cells, the
// way the CSV reader does, so a spreadsheet can be imported without saving
// it as CSV first. An .xlsx is a zip of XML files: the zip is read here and
// its compressed parts are inflated by the browser (DecompressionStream).
// Dates come out as 2026-11-22; numbers as Excel stores them.

interface Entry { method: number; size: number; offset: number }

/** The zip's entries by name. */
function entries(view: DataView<ArrayBuffer>): Map<string, Entry> {
  let end = -1;
  for (let i = view.byteLength - 22; i >= Math.max(0, view.byteLength - 66_000); i--) {
    if (view.getUint32(i, true) === 0x06054b50) { end = i; break; }
  }
  if (end < 0) throw new Error("not a zip");
  const count = view.getUint16(end + 10, true);
  let at = view.getUint32(end + 16, true);
  const out = new Map<string, Entry>();
  const decoder = new TextDecoder();
  for (let n = 0; n < count; n++) {
    if (view.getUint32(at, true) !== 0x02014b50) throw new Error("bad zip directory");
    const method = view.getUint16(at + 10, true);
    const size = view.getUint32(at + 20, true);
    const nameLen = view.getUint16(at + 28, true);
    const extraLen = view.getUint16(at + 30, true);
    const commentLen = view.getUint16(at + 32, true);
    const offset = view.getUint32(at + 42, true);
    const name = decoder.decode(new Uint8Array(view.buffer, view.byteOffset + at + 46, nameLen));
    out.set(name, { method, size, offset });
    at += 46 + nameLen + extraLen + commentLen;
  }
  return out;
}

async function readEntry(view: DataView<ArrayBuffer>, e: Entry): Promise<string> {
  if (view.getUint32(e.offset, true) !== 0x04034b50) throw new Error("bad zip entry");
  const start = e.offset + 30 + view.getUint16(e.offset + 26, true) + view.getUint16(e.offset + 28, true);
  const data = new Uint8Array(view.buffer, view.byteOffset + start, e.size);
  if (e.method === 0) return new TextDecoder().decode(data);
  if (e.method !== 8) throw new Error("unsupported compression");
  const body = new Response(data).body;
  if (!body) throw new Error("empty zip entry");
  return await new Response(body.pipeThrough(new DecompressionStream("deflate-raw"))).text();
}

function xml(text: string): Document {
  return new DOMParser().parseFromString(text, "application/xml");
}

/** Elements by local name, whatever the namespace prefix. */
function all(node: Document | Element, name: string): Element[] {
  return Array.from(node.getElementsByTagNameNS("*", name));
}

/** Which cell styles show dates (Excel keeps a date as a day number). */
function dateStyles(styles: Document | null): Set<number> {
  const out = new Set<number>();
  if (!styles) return out;
  const custom = new Map<number, string>();
  for (const f of all(styles, "numFmt")) custom.set(Number(f.getAttribute("numFmtId")), f.getAttribute("formatCode") ?? "");
  const xfs = all(styles, "cellXfs")[0];
  if (!xfs) return out;
  all(xfs, "xf").forEach((xf, i) => {
    const id = Number(xf.getAttribute("numFmtId") ?? "0");
    const code = (custom.get(id) ?? "").replace(/"[^"]*"|\[[^\]]*\]/g, "").toLowerCase();
    // Built-in date formats: 14-17 and 22, and the East Asian ones (27-36, 50-58).
    if ((id >= 14 && id <= 17) || id === 22 || (id >= 27 && id <= 36) || (id >= 50 && id <= 58) || (/[dy]/.test(code) && !/^[hms:. ]*$/.test(code))) out.add(i);
  });
  return out;
}

/** An Excel day number (days since 30 Dec 1899) as 2026-11-22. */
export function excelDay(serial: number, date1904 = false): string {
  // 1900 system: day 1 is 1 Jan 1900, and Excel counts a 29 Feb 1900 that
  // never was (day 60), so from day 61 on the days start a day earlier.
  // 1904 system (older Mac workbooks): day 0 is 1 Jan 1904.
  const start = date1904 ? Date.UTC(1904, 0, 1) : serial < 61 ? Date.UTC(1899, 11, 31) : Date.UTC(1899, 11, 30);
  const d = new Date(start + Math.round(serial * 86_400_000));
  return Number.isNaN(d.getTime()) || serial < 0 ? String(serial) : d.toISOString().slice(0, 10);
}

/** A string item's text: its own text and its runs', not phonetic readings (rPh). */
function textOf(item: Element): string {
  return all(item, "t").filter((t) => t.parentElement?.localName !== "rPh").map((t) => t.textContent).join("");
}

/** The column index of a cell reference: A1 is 0, AA7 is 26. */
function columnOf(ref: string): number {
  let n = 0;
  for (const ch of ref.replace(/[0-9]/g, "")) n = n * 26 + (ch.charCodeAt(0) - 64);
  return n - 1;
}

/** Reads the first sheet of a workbook into rows. Empty rows are dropped. */
export async function readXlsx(buffer: ArrayBuffer): Promise<string[][]> {
  const view = new DataView(buffer);
  const files = entries(view);
  const read = async (name: string) => {
    const e = files.get(name);
    return e ? xml(await readEntry(view, e)) : null;
  };
  const workbook = await read("xl/workbook.xml");
  const rels = await read("xl/_rels/workbook.xml.rels");
  let sheetPath = "xl/worksheets/sheet1.xml";
  const first = workbook ? all(workbook, "sheet")[0] : undefined;
  const rid = first?.getAttributeNS("http://schemas.openxmlformats.org/officeDocument/2006/relationships", "id") ?? first?.getAttribute("r:id");
  const target = rels && rid ? all(rels, "Relationship").find((r) => r.getAttribute("Id") === rid)?.getAttribute("Target") : undefined;
  if (target) sheetPath = target.startsWith("/") ? target.slice(1) : `xl/${target.replace(/^\.\//, "")}`;
  const sheet = await read(sheetPath);
  if (!sheet) throw new Error("no worksheet");
  const shared = (await read("xl/sharedStrings.xml"));
  const strings = shared ? all(shared, "si").map(textOf) : [];
  const date1904 = workbook ? all(workbook, "workbookPr").some((p) => ["1", "true"].includes(p.getAttribute("date1904") ?? "")) : false;
  const dates = dateStyles(await read("xl/styles.xml"));

  const rows: string[][] = [];
  for (const row of all(sheet, "row")) {
    const cells: string[] = [];
    let next = 0;
    for (const c of all(row, "c")) {
      const ref = c.getAttribute("r");
      const col = ref ? columnOf(ref) : next;
      next = col + 1;
      const type = c.getAttribute("t") ?? "n";
      const v = all(c, "v")[0]?.textContent ?? "";
      let text: string;
      if (type === "s") text = strings[Number(v)] ?? "";
      else if (type === "inlineStr") text = all(c, "is").map(textOf).join("");
      else if (type === "b") text = v === "1" ? "TRUE" : "FALSE";
      else if (type === "n" && v !== "" && dates.has(Number(c.getAttribute("s") ?? "-1"))) text = excelDay(Number(v), date1904);
      else text = v;
      while (cells.length < col) cells.push("");
      cells[col] = text;
    }
    const at = Number(row.getAttribute("r") ?? rows.length + 1) - 1;
    while (rows.length < at) rows.push([]);
    rows[at] = cells;
  }
  return rows.filter((r) => r.some((c) => c.trim() !== ""));
}
