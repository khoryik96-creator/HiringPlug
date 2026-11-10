// Finding CVs already in HiringPlug before uploading: each file's SHA-256 is
// worked out here, then the API says which are exactly a CV already there
// (skip them) and which share a CV's file name (ask first).
import { api, type Schemas, unwrap } from "../api/client";

export type MatchResult = Schemas["DocumentMatchResult"];

/** A file's SHA-256 in hex, as HiringPlug keeps it. */
export async function sha256Of(file: Blob): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", await file.arrayBuffer());
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** What is already there for each file: one candidate's CVs, or (no candidateId) any you may see. */
export async function findMatches(files: File[], candidateId?: string): Promise<MatchResult[]> {
  const refs = await Promise.all(files.map(async (f) => ({ fileName: f.name, sha256: await sha256Of(f) })));
  return unwrap(await api.POST("/v1/document-matches", { body: { files: refs, ...(candidateId ? { candidateId } : {}) } })).items;
}

/** "Nurul Huda's CV (cv.pdf, added 3 Nov 2026)". */
export function matchText(m: Schemas["DocumentMatch"]): string {
  const added = new Date(m.createdAt).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
  return `${m.candidateName}'s CV (${m.fileName}, added ${added})`;
}
