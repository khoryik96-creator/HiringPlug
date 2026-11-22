import { describe, expect, it } from "vitest";

import { excelDay, readXlsx } from "./xlsx";

const crcTable = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});
function crc32(data: Uint8Array): number {
  let c = 0xffffffff;
  for (const b of data) c = (crcTable[(c ^ b) & 0xff] ?? 0) ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

async function deflate(data: Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>> {
  const body = new Response(data).body;
  if (!body) throw new Error("no body");
  return new Uint8Array(await new Response(body.pipeThrough(new CompressionStream("deflate-raw"))).arrayBuffer());
}

/** A zip of the given files, as Excel writes it (compressed), or stored. */
export async function zip(files: Record<string, string>, compress = true): Promise<ArrayBuffer> {
  const enc = new TextEncoder();
  const parts: Uint8Array<ArrayBuffer>[] = [];
  const central: Uint8Array<ArrayBuffer>[] = [];
  let offset = 0;
  for (const [name, text] of Object.entries(files)) {
    const raw = enc.encode(text);
    const data = compress ? await deflate(raw) : raw;
    const nameBytes = enc.encode(name);
    const local = new DataView(new ArrayBuffer(30));
    local.setUint32(0, 0x04034b50, true);
    local.setUint16(8, compress ? 8 : 0, true);
    local.setUint32(14, crc32(raw), true);
    local.setUint32(18, data.length, true);
    local.setUint32(22, raw.length, true);
    local.setUint16(26, nameBytes.length, true);
    const dir = new DataView(new ArrayBuffer(46));
    dir.setUint32(0, 0x02014b50, true);
    dir.setUint16(10, compress ? 8 : 0, true);
    dir.setUint32(16, crc32(raw), true);
    dir.setUint32(20, data.length, true);
    dir.setUint32(24, raw.length, true);
    dir.setUint16(28, nameBytes.length, true);
    dir.setUint32(42, offset, true);
    parts.push(new Uint8Array(local.buffer), nameBytes, data);
    central.push(new Uint8Array(dir.buffer), nameBytes);
    offset += 30 + nameBytes.length + data.length;
  }
  const size = central.reduce((n, p) => n + p.length, 0);
  const end = new DataView(new ArrayBuffer(22));
  end.setUint32(0, 0x06054b50, true);
  end.setUint16(8, Object.keys(files).length, true);
  end.setUint16(10, Object.keys(files).length, true);
  end.setUint32(12, size, true);
  end.setUint32(16, offset, true);
  return await new Blob([...parts, ...central, new Uint8Array(end.buffer)]).arrayBuffer();
}

const NS = 'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"';

/** A workbook whose first sheet is "People" (stored as sheet2.xml, as after reordering sheets). */
export function workbook(): Record<string, string> {
  return {
    "xl/workbook.xml": `<workbook ${NS}><sheets><sheet name="People" sheetId="2" r:id="rId7"/><sheet name="Other" sheetId="1" r:id="rId1"/></sheets></workbook>`,
    "xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
      <Relationship Id="rId1" Type="worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId7" Type="worksheet" Target="worksheets/sheet2.xml"/></Relationships>`,
    "xl/sharedStrings.xml": `<sst ${NS}><si><t>First Name</t></si><si><t>Last Name</t></si><si><t>Available from</t></si><si><t>Phone</t></si>
      <si><r><t>Siti</t></r><r><rPr><b/></rPr><t xml:space="preserve"> Nur</t></r></si><si><t>Tan &amp; Co</t></si></sst>`,
    "xl/styles.xml": `<styleSheet ${NS}><numFmts count="1"><numFmt numFmtId="165" formatCode="dd/mm/yyyy"/></numFmts>
      <cellXfs count="3"><xf numFmtId="0"/><xf numFmtId="14"/><xf numFmtId="165"/></cellXfs></styleSheet>`,
    "xl/worksheets/sheet1.xml": `<worksheet ${NS}><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Wrong sheet</t></is></c></row></sheetData></worksheet>`,
    "xl/worksheets/sheet2.xml": `<worksheet ${NS}><sheetData>
      <row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c><c r="D1" t="s"><v>3</v></c></row>
      <row r="2"><c r="A2" t="s"><v>4</v></c><c r="B2" t="s"><v>5</v></c><c r="C2" s="2"><v>46348</v></c><c r="D2"><v>60123456789</v></c></row>
      <row r="4"><c r="A4" t="inlineStr"><is><t>Wei</t></is></c><c r="C4" s="1"><v>46349</v></c><c r="E4" t="b"><v>1</v></c></row>
      <row r="5"><c r="A5" t="str"><v></v></c></row>
    </sheetData></worksheet>`,
  };
}

describe("readXlsx", () => {
  it("reads the first sheet: shared and inline text, numbers, dates, gaps", async () => {
    const rows = await readXlsx(await zip(workbook()));
    expect(rows).toEqual([
      ["First Name", "Last Name", "Available from", "Phone"],
      ["Siti Nur", "Tan & Co", "2026-11-22", "60123456789"],
      ["Wei", "", "2026-11-23", "", "TRUE"],
    ]);
  });

  it("reads a workbook whose parts are stored, not compressed", async () => {
    const rows = await readXlsx(await zip(workbook(), false));
    expect(rows[0]?.[0]).toBe("First Name");
  });

  it("refuses what is not a workbook", async () => {
    await expect(readXlsx(new TextEncoder().encode("Name,Email\n").buffer)).rejects.toThrow();
  });

  it("turns Excel day numbers into dates", () => {
    expect(excelDay(1)).toBe("1899-12-31");
    expect(excelDay(46348)).toBe("2026-11-22");
  });
});

describe("a workbook saved by a spreadsheet program", () => {
  it("reads LibreOffice's .xlsx, dates as dates", async () => {
    const { readFile } = await import("node:fs/promises");
    const file = await readFile(`${process.cwd()}/src/import/fixtures/libreoffice-sample.xlsx`); // tests run from web/
    const rows = await readXlsx(file.buffer.slice(file.byteOffset, file.byteOffset + file.byteLength));
    expect(rows).toEqual([
      ["First Name", "Last Name", "Email", "Available from", "Expected salary"],
      ["Nurul Aisyah", "Rahman", "nurul@example.com", "2026-11-22", "8500"],
      ["Tan", "Wei Ming", "", "2027-01-04", "12000.5"],
    ]);
  });
});
