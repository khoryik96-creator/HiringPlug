// Turns an audit entry into a sentence: "changed Current title from
// Accountant to Senior Accountant". Salary changes never carry amounts.
import type { ActivityEntry } from "../api/client";

/** "currentTitle" -> "current title"; a few fields get friendlier names. */
export function fieldName(field: string): string {
  const named: Record<string, string> = {
    workAuthorization: "work authorization", noticePeriod: "notice period", linkedinUrl: "LinkedIn",
    registrationNumber: "registration no.", companyId: "company", contactId: "hiring contact", workType: "work type",
    workArrangement: "where", reasonNote: "note",
  };
  return named[field] ?? field.replace(/([A-Z])/g, " $1").toLowerCase();
}

function show(value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value)) return value.length > 0 ? value.map(show).join(", ") : "—";
  if (typeof value === "object") {
    const v = value as Record<string, unknown>;
    if ("value" in v && "unit" in v) return `${String(v.value)} ${String(v.unit)}`;
    if ("status" in v) return String(v.status).replace("_", " ");
    return JSON.stringify(value);
  }
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return String(value);
  return JSON.stringify(value);
}

/** What happened, in words; null for entries the timeline shows another way (notes). */
export function describe(entry: ActivityEntry): string | null {
  const [, verb] = entry.action.split(".");
  switch (entry.action) {
    case "candidate.compensation_updated":
      return "changed the salary details";
    case "job.salary_updated":
      return "changed the salary budget";
    case "note.created":
      return null;
    case "note.deleted":
      return "deleted a note";
    case "candidate.merged": {
      const filled = Array.isArray(entry.changes.filled) ? (entry.changes.filled as unknown[]).map((f) => fieldName(String(f))) : [];
      const who = typeof entry.changes.duplicateName === "string" && entry.changes.duplicateName ? ` (${entry.changes.duplicateName})` : "";
      return `merged in a duplicate record${who}${filled.length > 0 ? `, taking ${filled.join(", ")} from it` : ""}`;
    }
    case "candidate.merged_into":
      return "merged this record into another one";
    case "candidate.filled_from_cv": {
      const fields = Object.keys(entry.changes).map(fieldName);
      return fields.length > 0 ? `filled in from their CV: ${fields.join(", ")}` : "read their CV";
    }
    case "contact.employment_check_started":
      return `checked their LinkedIn profile (${show(entry.changes.fileName)})`;
    case "contact.employment_checked":
      return entry.changes.status !== "read"
        ? "could not read their LinkedIn profile"
        : `read their LinkedIn profile${"linkedinUrl" in entry.changes ? " and added their LinkedIn address" : ""}`;
  }
  if (verb === "created") return "added this record";
  // Entries are written by many modules over time: any shape must read, never break the page.
  const changes = Object.entries(entry.changes)
    .map(([field, change]) => {
      if (change === null || typeof change !== "object" || Array.isArray(change)) return `${fieldName(field)}: ${show(change)}`;
      const c = change as { from?: unknown; to?: unknown; redacted?: boolean };
      if (c.redacted) return `changed ${fieldName(field)}`;
      if (!("from" in c) && !("to" in c)) return `changed ${fieldName(field)}`;
      return `changed ${fieldName(field)} from ${show(c.from)} to ${show(c.to)}`;
    });
  return changes.length > 0 ? changes.join("; ") : verb ?? entry.action;
}
