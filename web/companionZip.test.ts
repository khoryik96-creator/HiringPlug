// @vitest-environment node
import { inflateRawSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import { companionBundle, companionFiles, crc32, zip } from "./companionZip";

const dir = new URL("../extension/linkedin-companion/", import.meta.url).pathname;

/** Reads a zip back by its central directory: name → contents (checked against its CRC). */
function unzip(z: Buffer): Map<string, Buffer> {
  const end = z.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  const count = z.readUInt16LE(end + 10);
  let at = z.readUInt32LE(end + 16);
  const out = new Map<string, Buffer>();
  for (let i = 0; i < count; i++) {
    expect(z.readUInt32LE(at)).toBe(0x02014b50);
    const crc = z.readUInt32LE(at + 16), size = z.readUInt32LE(at + 20), nameLength = z.readUInt16LE(at + 28), offset = z.readUInt32LE(at + 42);
    const name = z.subarray(at + 46, at + 46 + nameLength).toString("utf8");
    const dataAt = offset + 30 + z.readUInt16LE(offset + 26) + z.readUInt16LE(offset + 28);
    const data = inflateRawSync(z.subarray(dataAt, dataAt + size));
    expect(crc32(data), name).toBe(crc);
    out.set(name, data);
    at += 46 + nameLength;
  }
  return out;
}

describe("the companion download", () => {
  it("holds the extension's files in one folder, without its tests", () => {
    const files = unzip(zip(companionFiles(dir)));
    const names = [...files.keys()];
    expect(names).toContain("hiringplug-linkedin-companion/manifest.json");
    expect(names).toContain("hiringplug-linkedin-companion/worker.js");
    expect(names.some((n) => n.includes("/test/")), "tests stay out").toBe(false);
    const manifest = JSON.parse(files.get("hiringplug-linkedin-companion/manifest.json")?.toString() ?? "{}") as { manifest_version?: number };
    expect(manifest.manifest_version).toBe(3);
  });

  it("is also served as the bundle the companion updates itself from: the same files, at its manifest's version", () => {
    const bundle = companionBundle(dir);
    const zipped = [...companionFiles(dir).keys()].map((n) => n.replace("hiringplug-linkedin-companion/", "")).sort();
    expect(Object.keys(bundle.files).sort()).toEqual(zipped);
    const manifest = JSON.parse(bundle.files["manifest.json"] ?? "{}") as { version?: string; name?: string };
    expect(bundle.version).toBe(manifest.version);
    // What installed companions' updaters accept: the listed name changes to HyPlug's
    // once every companion takes either name (1.7.0 onwards).
    expect(manifest.name).toBe("HiringPlug LinkedIn Companion");
    expect(Object.keys(bundle.files)).toContain("update.html");
    // Every path is one the updater writes (extension/linkedin-companion/updater.js).
    for (const p of Object.keys(bundle.files)) expect(p).toMatch(/^[a-z0-9_-]+(\/[a-z0-9_-]+)*\.(js|json|html|css|md|txt)$/i);
  });

  it("is the same bytes each build", () => {
    expect(zip(companionFiles(dir)).equals(zip(companionFiles(dir)))).toBe(true);
  });

  it("checks contents the standard way", () => {
    expect(crc32(Buffer.from("123456789"))).toBe(0xcbf43926);
  });
});
