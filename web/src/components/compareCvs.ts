// What two CV versions of one candidate disagree on, from their parsed
// details: contact fields, jobs (matched by company) and skills.
import type { Schemas } from "../api/client";
import { countryName, formatPeriod } from "../labels";
import { latestJob } from "./cvSuggestions";

type Profile = Schemas["ParsedProfile"];
type Job = Profile["experience"][number];

export interface FieldRow {
  label: string;
  a: string;
  b: string;
  differs: boolean;
}

const tidy = (s: string) => s.toLowerCase().replace(/\s+/g, " ").trim();
const digits = (s: string) => s.replace(/\D/g, "");

/** Field by field. A value only one CV has is a difference too. */
export function compareFields(a: Profile | null, b: Profile | null): FieldRow[] {
  const rows: [string, (p: Profile) => string, (s: string) => string][] = [
    ["Name", (p) => p.fullName?.value ?? "", tidy],
    ["Email", (p) => p.email?.value ?? "", tidy],
    ["Phone", (p) => p.phone?.value ?? "", digits],
    ["LinkedIn", (p) => p.linkedinUrl?.value ?? "", (s) => tidy(s).replace(/^https?:\/\/(www\.)?/, "")],
    ["Location", (p) => p.location?.value ?? "", tidy],
    ["Nationality", (p) => (p.nationality ? countryName(p.nationality.value) : ""), tidy],
    ["Notice period", (p) => p.noticePeriod?.value ?? "", tidy],
    ["Current title", (p) => latestJob(p)?.title?.value ?? "", tidy],
    ["Current company", (p) => latestJob(p)?.company?.value ?? "", tidy],
  ];
  return rows.map(([label, get, key]) => {
    const av = a ? get(a) : "";
    const bv = b ? get(b) : "";
    return { label, a: av, b: bv, differs: key(av) !== key(bv) };
  });
}

export const describeJob = (j: Job) =>
  `${[j.title?.value, j.company?.value].filter(Boolean).join(" at ")} (${formatPeriod(j.startDate, j.endDate, j.current) || "no dates"})`;

const jobKey = (j: Job) => tidy(j.company?.value ?? j.title?.value ?? "").replace(/\b(sdn|bhd|berhad|pte|ltd|inc)\b\.?/g, "").trim();

export interface JobDiff {
  onlyA: string[];
  onlyB: string[];
  /** Same employer, different title or dates: the kind of change a formatted CV must not make. */
  changed: { a: string; b: string }[];
}

export function compareJobs(a: Profile | null, b: Profile | null): JobDiff {
  const left = a?.experience ?? [];
  const right = [...(b?.experience ?? [])];
  const diff: JobDiff = { onlyA: [], onlyB: [], changed: [] };
  for (const job of left) {
    const i = right.findIndex((r) => jobKey(r) === jobKey(job));
    if (i < 0) {
      diff.onlyA.push(describeJob(job));
      continue;
    }
    const [match] = right.splice(i, 1);
    if (match && describeJob(match).toLowerCase() !== describeJob(job).toLowerCase()) {
      diff.changed.push({ a: describeJob(job), b: describeJob(match) });
    }
  }
  diff.onlyB = right.map(describeJob);
  return diff;
}

export function compareSkills(a: Profile | null, b: Profile | null): { onlyA: string[]; onlyB: string[] } {
  const as = (a?.skills ?? []).map((s) => s.value);
  const bs = (b?.skills ?? []).map((s) => s.value);
  const has = (list: string[], v: string) => list.some((x) => tidy(x) === tidy(v));
  return { onlyA: as.filter((v) => !has(bs, v)), onlyB: bs.filter((v) => !has(as, v)) };
}
