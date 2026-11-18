// Ad blockers (uBlock Origin, AdBlock, Brave) hide any element whose class
// looks like an advert ("ad-text", "adv-box", "advert", "sponsor"...): the
// blind ad's text vanished for a recruiter running one. No class or id in the
// app may look like that.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { expect, it } from "vitest";

const SRC = join(process.cwd(), "src/"); // tests run from web/
const files = (dir: string): string[] => readdirSync(dir).flatMap((name) => {
  const path = join(dir, name);
  return statSync(path).isDirectory() ? files(path) : /\.(tsx|css)$/.test(name) && !name.includes(".test.") ? [path] : [];
});
// Names block lists hide on any site: ad-…, ads-…, adv-…, advert…, sponsor…, banner-ad, adbox…
const BLOCKED = /^(ads?|adv|advert\w*|sponsor\w*|adbox\w*|adbanner\w*|banner-ad\w*)([-_]|$)/i;

it("uses no class or id an ad blocker would hide", () => {
  const found: string[] = [];
  for (const file of files(SRC)) {
    const text = readFileSync(file, "utf8");
    const names = file.endsWith(".css")
      ? [...text.matchAll(/[.#](-?[_a-zA-Z][\w-]*)/g)].map((m) => m[1] ?? "")
      : [...text.matchAll(/(?:className|id)=(?:"([^"]*)"|\{([^}]*(?:\{[^}]*\}[^}]*)*)\})/g)].flatMap((m) =>
        // A plain value, or every string in an expression (`a ${x ? "b" : "c"}`, cond ? "x" : "y").
        (m[1] !== undefined ? [m[1]] : [...(m[2] ?? "").matchAll(/["'`]([^"'`$]*)/g)].map((q) => q[1] ?? "")).flatMap((v) => v.split(/\s+/)));
    for (const name of names) if (BLOCKED.test(name)) found.push(`${file.slice(SRC.length)}: ${name}`);
  }
  expect(found).toEqual([]);
});
