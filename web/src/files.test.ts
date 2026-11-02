import { describe, expect, it } from "vitest";

// Every file under src, as paths like "./components/Field.tsx".
const files = Object.keys(import.meta.glob("./**/*"));

describe("file names", () => {
  it("never differ only in case or extension within a folder", () => {
    // On macOS and Windows "CandidateForm.tsx" and "candidateForm.ts" are the
    // same name to the disk, and an import of "./CandidateForm" finds the .ts
    // file first: the app stops compiling there while Linux CI stays green.
    const byName = new Map<string, string[]>();
    for (const file of files) {
      const key = file.replace(/\.[^./]+$/, "").toLowerCase();
      byName.set(key, [...(byName.get(key) ?? []), file]);
    }
    const clashes = [...byName.values()].filter((group) => group.length > 1);
    expect(clashes).toEqual([]);
  });

  it("finds the files it checks", () => {
    expect(files).toContain("./components/CandidateForm.tsx");
  });
});
