// An audit log entry in words: "Changed a candidate", "Moved to Interview",
// "Exported candidates (212)". About 290 actions are written by the modules;
// a few have their own sentence and the rest are built from the record kind
// and the verb ("candidate.updated" -> "Changed a candidate"). Never breaks
// on a shape it does not know.
import type { Schemas } from "../../api/client";
import { describe, fieldName, show } from "../../components/activityText";
import { formatNotice, NOTICE_UNIT, STAGE } from "../../labels";

export type AuditEntry = Schemas["AuditEntry"];

/** Record kinds with a nicer name than their id ("job_ad" -> "job ad" needs none). */
const NOUN: Record<string, string> = {
  user: "team member", tenant: "company", job_order: "job", application: "candidate on a job", jd_file: "job description",
  auditlog: "audit log", assistant: "AI assistant", aiprovider: "AI provider", ai_usage: "AI usage", cv: "formatted CV", jd: "job description",
};

/** Past-tense verbs in action names, in plain words. */
const VERB: Record<string, string> = {
  created: "Added", added: "Added", updated: "Changed", changed: "Changed", edited: "Changed", saved: "Saved", set: "Set",
  deleted: "Deleted", removed: "Removed", restored: "Restored", exported: "Exported", imported: "Imported", merged: "Merged",
  renamed: "Renamed", shared: "Shared", unshared: "Stopped sharing", uploaded: "Uploaded", downloaded: "Downloaded",
  sent: "Sent", queued: "Sent", scheduled: "Scheduled", rescheduled: "Rescheduled", cancelled: "Cancelled", moved: "Moved",
  archived: "Archived", approved: "Approved", rejected: "Rejected", requested: "Asked for", reset: "Reset", invited: "Invited",
  connected: "Connected", disconnected: "Disconnected", recorded: "Recorded", opened: "Opened", closed: "Closed",
  reopened: "Reopened", revoked: "Revoked", drafted: "Drafted", accepted: "Accepted", declined: "Declined",
  withdrawn: "Withdrew", enabled: "Turned on", disabled: "Turned off", checked: "Checked", formatted: "Formatted",
  parsed: "Read", started: "Started", finished: "Finished", completed: "Finished", published: "Published",
  unpublished: "Unpublished", assigned: "Assigned", paid: "Marked as paid", invoiced: "Invoiced", submitted: "Submitted",
};

/** Actions whose sentence the record timeline already words well (activityText's describe). */
const TIMELINE = new Set([
  "candidate.screened", "candidate.compensation_updated", "job.salary_updated", "note.deleted", "candidate.merged",
  "candidate.merged_into", "candidate.filled_from_cv", "email.shortlist_sent", "client_shortlist.created",
  "client_shortlist.shared", "client_shortlist.opened", "client_shortlist.revoked", "client_shortlist.answered",
  "job.jd_uploaded", "job.jd_deleted", "job.jd_kept", "contact.employment_check_started", "contact.employment_checked",
]);

/** Verbs that, on a setting, mean it was changed (the rest are things done in Settings, e.g. a backup downloaded). */
const SETTING_VERBS = new Set(["changed", "updated", "saved", "set", "reset", "enabled", "disabled"]);

const capital = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const words = (s: string) => s.replace(/_/g, " ");
const noun = (kind: string) => NOUN[kind] ?? words(kind);
const article = (n: string) => (/^[aeiou]/i.test(n) ? `an ${n}` : `a ${n}`);
const plural = (n: string) => (/[^aeiou]y$/.test(n) ? `${n.slice(0, -1)}ies` : `${n}s`);
const num = (v: unknown) => (typeof v === "number" ? v : null);

/** "What" in the table: one short sentence. */
export function whatHappened(entry: Pick<AuditEntry, "action" | "changes">): string {
  const { action, changes } = entry;
  const dot = action.indexOf(".");
  const kind = dot < 0 ? action : action.slice(0, dot);
  const verb = dot < 0 ? "" : action.slice(dot + 1);
  switch (action) {
    case "user.signed_in": return "Signed in";
    case "user.signed_out": return "Signed out";
    case "note.created": return "Added a note";
    case "application.created": return "Added to a job";
    case "application.moved": {
      const stage = (changes.stage as { to?: unknown } | undefined)?.to;
      const label = typeof stage === "string" ? (STAGE as Record<string, string>)[stage] ?? words(stage) : null;
      return label ? `Moved to ${label}` : "Moved on the pipeline";
    }
    case "email.queued": return changes.salary === true ? "Sent an email with salary" : "Sent an email";
    case "email.failed": return "An email could not be sent";
    case "document.cv_parsed": return "Read a CV";
    case "document.text_extracted": return "Read a document's text";
    case "cv.formatted": return "Formatted a CV";
    case "role.access_changed": return "Changed what a role may do";
    case "role.access_reset": return "Reset a role's access";
    case "assistant.asked": return "Asked the AI assistant";
    case "assistant.answered": return "The AI assistant answered";
    case "match.scored": return "Scored a candidate's fit";
    case "auditlog.exported": return "Downloaded the audit log";
    case "candidate.updated_from_cv": return "Updated a candidate from their CV";
    case "folder.items_added": return "Added to a folder";
    case "folder.items_removed": return "Removed from a folder";
    case "tenant.renamed": return "Renamed the company";
    case "placement.backed_out": return "Recorded that a placement backed out";
    case "job_ad.blind_rewritten": return "Rewrote a job ad without the client's name";
  }
  if (TIMELINE.has(action)) {
    const said = describe(entry);
    if (said) return capital(said);
  }
  if (verb === "exported") {
    const count = num(changes.count);
    return `Exported ${plural(noun(kind))}${count === null ? "" : ` (${count.toLocaleString("en")})`}`;
  }
  const parts = verb.split("_");
  const last = parts.pop() ?? "";
  const object = parts.join(" ");
  const done = VERB[last];
  if (kind.endsWith("_settings") && done) return `${done} the ${noun(kind)}`;
  if (kind === "settings" && done) {
    // "settings.salary_calculation_changed" is a setting; "settings.backup_downloaded" is a thing done there.
    return SETTING_VERBS.has(last) ? `${done} the ${object} settings` : `${done} ${article(object)}`;
  }
  if (VERB[verb]) return `${VERB[verb]} ${article(noun(kind))}`;
  if (done && object === "settings") return `${done} the ${noun(kind)} settings`;
  if (done && object) return `${done} ${article(noun(kind))}'s ${object}`;
  return `${capital(noun(kind))}: ${words(verb || action)}`;
}

/** One line of what changed: a field from one value to another, a hidden change, or a detail. */
export type ChangeLine =
  | { field: string; kind: "change"; from: string; to: string }
  | { field: string; kind: "hidden"; amount: boolean }
  | { field: string; kind: "detail"; value: string };

/** Hidden fields that are money (the others, like an email's subject, are hidden because they may show it). */
const AMOUNT = /salary|compensation|money|fee|pay/i;

function value(v: unknown): string {
  if (typeof v === "boolean") return v ? "yes" : "no";
  if (typeof v === "string" && Object.hasOwn(STAGE, v)) return (STAGE as Record<string, string>)[v] ?? v;
  const n = v as { value?: unknown; unit?: unknown } | null;
  if (n && typeof n === "object" && typeof n.value === "number" && typeof n.unit === "string" && Object.hasOwn(NOTICE_UNIT, n.unit)) {
    return formatNotice(n as Schemas["NoticePeriod"]);
  }
  return show(v);
}

/** The lines shown when a row is opened: field -> from/to; salary and other hidden fields only as "changed". */
export function changeLines(changes: AuditEntry["changes"]): ChangeLine[] {
  return Object.entries(changes).map(([key, change]): ChangeLine => {
    const field = capital(fieldName(key));
    if (change === null || typeof change !== "object" || Array.isArray(change)) return { field, kind: "detail", value: value(change) };
    const c = change as { from?: unknown; to?: unknown; redacted?: unknown; changed?: unknown };
    if (c.redacted === true) return { field, kind: "hidden", amount: AMOUNT.test(key) };
    if ("from" in c || "to" in c) return { field, kind: "change", from: value(c.from), to: value(c.to) };
    if (c.changed === true) return { field, kind: "hidden", amount: AMOUNT.test(key) };
    return { field, kind: "detail", value: value(change) };
  });
}

/** The Record column for an export: which list it was ("Candidate list"), not who exported it. */
export function exportedList(action: string): string | null {
  const [kind = "", verb] = action.split(".");
  return verb === "exported" ? `${capital(noun(kind))} list` : null;
}

/** Where a record opens in the app (candidates, jobs, companies and contacts; a candidate on a job opens the candidate); null for the others. */
export function recordHref(entry: Pick<AuditEntry, "entityType" | "entityId" | "candidateId">): string | null {
  if (entry.entityType === "application") return entry.candidateId ? `#/candidates/${entry.candidateId}` : null;
  const base: Record<string, string> = { candidate: "candidates", job: "jobs", job_order: "jobs", company: "companies", contact: "contacts" };
  const at = base[entry.entityType];
  return at && /^[a-z]+_[0-9a-z]+$/.test(entry.entityId) ? `#/${at}/${entry.entityId}` : null;
}
