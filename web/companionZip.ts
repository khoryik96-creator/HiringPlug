// The LinkedIn companion, zipped for "Download the companion" (Settings ›
// LinkedIn companion): extension/linkedin-companion, less its tests, in a
// folder the browser's "Load unpacked" opens. Built with the web app (and
// served by `vite` while developing), so the download is always the version
// in this checkout.
import { readdirSync, readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { deflateRawSync } from "node:zlib";
import type { Plugin } from "vite";

export const COMPANION_ZIP = "linkedin-companion.zip";
const FOLDER = "hiringplug-linkedin-companion";

/** The companion's files (path inside the zip → contents), tests and hidden files left out. */
export function companionFiles(dir: string): Map<string, Buffer> {
  const out = new Map<string, Buffer>();
  const walk = (d: string) => {
    for (const e of readdirSync(d, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      if (e.name.startsWith(".") || (e.isDirectory() && e.name === "test")) continue;
      const path = join(d, e.name);
      if (e.isDirectory()) walk(path);
      else if (e.isFile()) out.set(`${FOLDER}/${relative(dir, path).split(sep).join("/")}`, readFileSync(path));
    }
  };
  walk(dir);
  return out;
}

const CRC_TABLE = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});

export function crc32(data: Uint8Array): number {
  let c = 0xffffffff;
  for (const byte of data) c = (CRC_TABLE[(c ^ byte) & 0xff] ?? 0) ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

/** A zip of the files (deflated, UTF-8 names, a fixed date so each build gives the same bytes). */
export function zip(files: Map<string, Buffer>): Buffer {
  const date = ((2026 - 1980) << 9) | (1 << 5) | 1; // 1 January 2026
  const local: Buffer[] = [];
  const central: Buffer[] = [];
  let offset = 0;
  for (const [name, data] of files) {
    const nameBytes = Buffer.from(name, "utf8");
    const packed = deflateRawSync(data, { level: 9 });
    const crc = crc32(data);
    const head = Buffer.alloc(30);
    head.writeUInt32LE(0x04034b50, 0); head.writeUInt16LE(20, 4); head.writeUInt16LE(0x0800, 6); head.writeUInt16LE(8, 8);
    head.writeUInt16LE(0, 10); head.writeUInt16LE(date, 12); head.writeUInt32LE(crc, 14);
    head.writeUInt32LE(packed.length, 18); head.writeUInt32LE(data.length, 22); head.writeUInt16LE(nameBytes.length, 26); head.writeUInt16LE(0, 28);
    const entry = Buffer.alloc(46);
    entry.writeUInt32LE(0x02014b50, 0); entry.writeUInt16LE(20, 4); entry.writeUInt16LE(20, 6); entry.writeUInt16LE(0x0800, 8); entry.writeUInt16LE(8, 10);
    entry.writeUInt16LE(0, 12); entry.writeUInt16LE(date, 14); entry.writeUInt32LE(crc, 16); entry.writeUInt32LE(packed.length, 20);
    entry.writeUInt32LE(data.length, 24); entry.writeUInt16LE(nameBytes.length, 28); entry.writeUInt32LE(offset, 42);
    local.push(head, nameBytes, packed);
    central.push(entry, nameBytes);
    offset += head.length + nameBytes.length + packed.length;
  }
  const dir = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(files.size, 8); end.writeUInt16LE(files.size, 10);
  end.writeUInt32LE(dir.length, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...local, dir, end]);
}

/** Serves /linkedin-companion.zip while developing and adds it to the built app. */
export function companionZip(dir: string): Plugin {
  const build = () => zip(companionFiles(dir));
  return {
    name: "hiringplug-companion-zip",
    configureServer(server) {
      server.middlewares.use(`/${COMPANION_ZIP}`, (_req, res) => {
        res.setHeader("Content-Type", "application/zip");
        res.end(build());
      });
    },
    generateBundle() {
      this.emitFile({ type: "asset", fileName: COMPANION_ZIP, source: build() });
    },
  };
}
