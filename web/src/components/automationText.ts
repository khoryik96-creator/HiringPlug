// Automations in words: "WHEN a candidate moves to Rejected · 2 days after ·
// email the candidate", and the editor's form state (pure, unit tested).
import type { Schemas } from "../api/client";
import { STAGE } from "../labels";

type Automation = Schemas["Automation"];
type Trigger = Schemas["AutomationTrigger"];
type ActionKind = Schemas["AutomationActionKind"];

export const TRIGGER: Record<Trigger, string> = {
  application_received: "A candidate applies or is added to a job",
  stage_changed: "A candidate moves to a stage",
  interview_scheduled: "An interview is booked",
  placement_starts: "A placement starts",
  guarantee_ends: "A placement's guarantee ends",
  invoice_due: "A placement's invoice falls due (unpaid)",
};

export const ACTION: Record<ActionKind, string> = {
  email_candidate: "Email the candidate",
  email_client: "Email the client's hiring contact",
  email_recruiter: "Email the recruiter",
  task: "Give the recruiter a task",
};

export const RUN_STATUS: Record<Schemas["AutomationRunStatus"], string> = {
  scheduled: "Scheduled", to_approve: "To approve", done: "Done", skipped: "Skipped", failed: "Failed",
};

/** Whether a trigger has a date ahead (so "days before" makes sense). */
export const dated = (t: Trigger) => t !== "application_received" && t !== "stage_changed";

/** WHEN, in words. */
export function whenText(a: Pick<Automation, "trigger" | "stage">): string {
  if (a.trigger === "stage_changed") return `A candidate moves to ${a.stage ? STAGE[a.stage] : "a stage"}`;
  return TRIGGER[a.trigger];
}

/** When it runs, in words: "straight away", "2 days after", "1 day before". */
export function timingText(a: Pick<Automation, "trigger" | "offsetDays">): string {
  const n = Math.abs(a.offsetDays);
  const days = n === 1 ? "1 day" : `${String(n)} days`;
  if (a.offsetDays === 0) return dated(a.trigger) && a.trigger !== "interview_scheduled" ? "on the day" : "straight away";
  return a.offsetDays > 0 ? `${days} after` : `${days} before`;
}

/** THEN, in words: "email the candidate and give the recruiter a task". */
export function thenText(actions: Schemas["AutomationAction"][]): string {
  const parts = actions.map((x) => ACTION[x.kind].replace(/^./, (c) => c.toLowerCase()));
  return parts.length <= 1 ? (parts[0] ?? "") : `${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1] ?? ""}`;
}

/** A subject with what is known in the list filled in (the rest fills when sent). */
export function roughFill(text: string, r: Pick<Schemas["AutomationRun"], "candidateName" | "jobTitle" | "companyName">): string {
  const values: Record<string, string | null> = {
    "candidate.fullName": r.candidateName, "candidate.firstName": r.candidateName?.split(" ")[0] ?? null,
    "job.title": r.jobTitle, "company.name": r.companyName,
  };
  return text.replace(/\{\{([a-zA-Z.]+)\}\}/g, (all, key: string) => values[key] ?? all);
}

export type ActionRow = { kind: ActionKind; subject: string; body: string; title: string };

export type AutomationForm = {
  name: string;
  enabled: boolean;
  trigger: Trigger;
  stage: Schemas["Stage"];
  days: string; // how many
  direction: "after" | "before";
  companyId: string; // "" any client
  actions: ActionRow[];
  sendMode: Schemas["AutomationSendMode"];
};

export const blankAction = (kind: ActionKind = "email_candidate"): ActionRow => ({ kind, subject: "", body: "", title: "" });

export function automationForm(a: Automation | undefined, defaultMode: Schemas["AutomationSendMode"]): AutomationForm {
  if (!a) {
    return { name: "", enabled: true, trigger: "stage_changed", stage: "interview", days: "0", direction: "after", companyId: "", actions: [blankAction()], sendMode: defaultMode };
  }
  return {
    name: a.name, enabled: a.enabled, trigger: a.trigger, stage: a.stage ?? "interview",
    days: String(Math.abs(a.offsetDays)), direction: a.offsetDays < 0 ? "before" : "after", companyId: a.companyId ?? "",
    actions: a.actions.map((x) => ({ kind: x.kind, subject: x.subject ?? "", body: x.body ?? "", title: x.title ?? "" })),
    sendMode: a.sendMode,
  };
}

export function automationBody(f: AutomationForm, version?: number): Schemas["AutomationInput"] {
  const days = Number(f.days) || 0;
  return {
    ...(version ? { version } : {}),
    name: f.name.trim(),
    enabled: f.enabled,
    trigger: f.trigger,
    ...(f.trigger === "stage_changed" ? { stage: f.stage } : {}),
    offsetDays: f.direction === "before" && dated(f.trigger) ? -days : days,
    companyId: f.companyId || null,
    actions: f.actions.map((x) => (x.kind === "task" ? { kind: x.kind, title: x.title.trim() } : { kind: x.kind, subject: x.subject.trim(), body: x.body.trim() })),
    sendMode: f.sendMode,
  };
}

/** An automation switched on or off, everything else as it is. */
export function toggled(a: Automation, enabled: boolean): Schemas["AutomationInput"] {
  return {
    version: a.version, name: a.name, enabled, trigger: a.trigger, ...(a.stage ? { stage: a.stage } : {}), offsetDays: a.offsetDays,
    companyId: a.companyId, actions: a.actions, sendMode: a.sendMode,
  };
}
