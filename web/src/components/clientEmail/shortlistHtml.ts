// An email to the client about several candidates: the details each one
// brings (their own values, marked as fields so the email knows it shows
// salary), written as a block each or as a table, and the profile each
// attaches.
import type { Schemas } from "../../api/client";
import { escapeHtml } from "./richText";

export type Layout = "blocks" | "table";
export type CvChoice = Schemas["EmailCvAttachment"];

/** A detail a candidate's block or row can show: its field and how it is labelled. */
export type Detail = { id: string; token: string; key: string; label: string; salary?: boolean };

export const DETAILS: Detail[] = [
 {id:"currentTitle",token:"[Current title]",key:"candidate.currentTitle",label:"Current title"},
 {id:"industries",token:"[Industries]",key:"candidate.industries",label:"Industries"},
 {id:"currentBreakdown",token:"[Current salary breakdown]",key:"candidate.currentBreakdown",label:"Current breakdown",salary:true},
 {id:"expectedNotes",token:"[Expected salary notes]",key:"candidate.expectedNotes",label:"Expected salary notes",salary:true},
 {id:"salaryNotes",token:"[Salary notes]",key:"candidate.salaryNotes",label:"Salary notes",salary:true},
  { id: "currentSalary", token: "[Current salary]", key: "candidate.currentSalary", label: "Current salary", salary: true },
  { id: "expectedSalary", token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true },
  { id: "noticePeriod", token: "[Notice period]", key: "candidate.noticePeriod", label: "Notice period" },
  { id: "residence", token: "[Lives in]", key: "candidate.residence", label: "Location" },
  { id: "currentCompany", token: "[Current company]", key: "candidate.currentCompany", label: "Current company" },
  { id: "nationality", token: "[Nationality]", key: "candidate.nationality", label: "Nationality" },
  { id: "workAuthorization", token: "[Expat status]", key: "candidate.workAuthorization", label: "Expat status" },
];
export const DEFAULT_DETAILS = ["currentSalary", "expectedSalary", "noticePeriod", "residence"];

/** One candidate as the message shows them: their values by field token ("[Expected salary]"). */
export type Person = { id?:string; name: string; values: Record<string, string> };

const missing = (d: Detail) => `<span data-field="${d.key}" data-missing-detail="${escapeHtml(d.label)}" title="${escapeHtml(d.label)}: not on record">—</span>`;
const field = (d: Detail, value: string) => `<span data-field="${d.key}"${/\((?:unit not specified|component needs review|amount needs review)\)/.test(value) ? ' data-missing-detail="Review salary component" title="Review the salary component"' : ""}>${escapeHtml(value)}</span>`;

/** A block for each candidate: "1. Name, title" then "Label: value · Label: value". */
export function blocksHtml(people: Person[], details: Detail[]): string {
  return people.map((p, i) => {
    const facts = details.map((d) => {
      const v = p.values[d.token];
      return `${escapeHtml(d.label)}: ${v ? field(d, v) : missing(d)}`;
    }).join("<br>");
    return `<p${p.id?` data-candidate-id="${escapeHtml(p.id)}"`:""}><b>${String(i + 1)}. ${escapeHtml(p.name)}</b>${facts ? `<br>${facts}` : ""}</p>`;
  }).join("") + "<p><br></p>";
}

/** One table, a row for each candidate and a column for each detail. */
export function tableHtml(people: Person[], details: Detail[]): string {
  const cell = "border:1px solid #c8c8d0;padding:6px 8px;text-align:left;vertical-align:top";
  const head = ["Candidate", ...details.map((d) => d.label)].map((h) => `<th style="${cell};background:#f0edf9">${escapeHtml(h)}</th>`).join("");
  const rows = people.map((p) => {
    const who = escapeHtml(p.name);
    const cells = details.map((d) => {
      const v = p.values[d.token];
      return `<td style="${cell}">${v ? field(d, v) : missing(d)}</td>`;
    }).join("");
    return `<tr${p.id?` data-candidate-id="${escapeHtml(p.id)}"`:""}><td style="${cell}">${who}</td>${cells}</tr>`;
  }).join("");
  return `<table style="border-collapse:collapse"><thead><tr>${head}</tr></thead><tbody>${rows}</tbody></table><p><br></p>`;
}

/** What is not on record, to say before inserting: "Nurul Huda: Expected salary". */
export function missingDetails(people: Person[], details: Detail[]): string[] {
  return people.flatMap((p) => {
    const gaps = details.filter((d) => !p.values[d.token]).map((d) => d.label);
    return gaps.length > 0 ? [`${p.name}: ${gaps.join(", ")}`] : [];
  });
}

/** A profile a candidate can attach: a formatted CV (PDF once made, or Word) or another CV. */
export type Profile = { choice: CvChoice; label: string };

const SOURCE: Record<string, string> = { original: "Original", jobstreet: "JobStreet", linkedin: "LinkedIn", formatted: "Formatted", other: "Other" };

/** A candidate's profiles to choose from, formatted ones (newest first) first; the first is the default. */
export function profilesOf(docs: Schemas["Document"][]): Profile[] {
  const stamp=(d:Schemas["Document"])=>{const date=new Date(d.createdAt);return Number.isNaN(date.getTime())?"":` · ${date.toLocaleString()}`;};
  const newest = [...docs].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  const formatted = newest.filter((d) => d.source === "formatted").flatMap((d) => [
    ...(d.pdfStatus === "ready" ? [{ choice: { documentId: d.id, asPdf: true }, label: `${d.fileName.replace(/\.docx$/i, "")} (PDF)${stamp(d)}` }] : []),
    { choice: { documentId: d.id, asPdf: false }, label: `${d.fileName} (Word)${stamp(d)}` },
  ]);
  const others = newest.filter((d) => d.source !== "formatted")
    .map((d) => ({ choice: { documentId: d.id, asPdf: false }, label: `${SOURCE[d.source] ?? d.source}: ${d.fileName}${stamp(d)}` }));
  return [...formatted, ...others];
}

export const choiceKey = (c: CvChoice | null) => (c ? `${c.documentId}:${c.asPdf ? "pdf" : "file"}` : "");
