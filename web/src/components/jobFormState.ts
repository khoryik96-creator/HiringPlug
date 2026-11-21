// The job edit form's state, its checks, and its conversion to API requests.
import type { JobOrder, Schemas } from "../api/client";
import type { PickedCompany } from "./CompanyPicker";
import { orNull } from "./forms";

export interface JobFormState {
  company: PickedCompany | null;
  contactId: string;
  title: string;
  status: Schemas["JobStatus"];
  location: string;
  workType: "" | Schemas["WorkType"];
  workArrangement: "" | Schemas["WorkArrangement"];
  openings: string;
  description: string;
  country: string;
  intakeDate: string; // YYYY-MM-DD
}

/** Today's date where the user is, as YYYY-MM-DD. */
export function todayLocal(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${String(now.getFullYear())}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

/** Where the API reports a problem with each input (the VALIDATION_FAILED field path). */
export const JOB_PATHS: Record<keyof JobFormState, string> = {
  company: "companyId", contactId: "contactId", title: "title", status: "status", location: "location",
  workType: "workType", workArrangement: "workArrangement", openings: "openings", description: "description",
  country: "country", intakeDate: "intakeDate",
};

export function formFromJob(j: JobOrder | null, company: PickedCompany | null = null): JobFormState {
  return {
    company: j ? { id: j.companyId, name: j.companyName } : company,
    contactId: j?.contactId ?? "",
    title: j?.title ?? "",
    status: j?.status ?? "open",
    location: j?.location ?? "",
    workType: j?.workType ?? "",
    workArrangement: j?.workArrangement ?? "",
    openings: String(j?.openings ?? 1),
    description: j?.description ?? "",
    country: j?.country ?? "",
    intakeDate: j?.intakeDate ?? todayLocal(), // a new job's intake is today unless chosen
  };
}

/** Mistakes worth catching before sending; the API checks everything again. */
export function jobProblems(f: JobFormState): Record<string, string> {
  const problems: Record<string, string> = {};
  if (!f.company) problems[JOB_PATHS.company] = "Choose the company you are hiring for.";
  if (f.title.trim() === "") problems[JOB_PATHS.title] = "Enter the job title.";
  if (!/^[1-9]\d{0,2}$/.test(f.openings.trim())) problems[JOB_PATHS.openings] = "Enter how many people to hire (1–999).";
  return problems;
}

/** The fields of a create or update request: every field is sent; empty inputs clear it. */
export function jobFields(f: JobFormState) {
  return {
    companyId: f.company?.id ?? "",
    contactId: f.contactId === "" ? null : f.contactId,
    title: f.title.trim(),
    status: f.status,
    location: orNull(f.location),
    workType: f.workType === "" ? null : f.workType,
    workArrangement: f.workArrangement === "" ? null : f.workArrangement,
    openings: Number(f.openings.trim()),
    description: orNull(f.description),
    country: f.country === "" ? null : f.country,
    ...(f.intakeDate ? { intakeDate: f.intakeDate } : {}),
  };
}

/**
 * The form with what a job description says, where the form is still empty
 * (what someone typed is kept).
 */
/** The fields a JD fills in, with what it put there. */
export type JdFilled = Partial<Record<"title" | "location" | "country" | "workType" | "workArrangement" | "description" | "openings", string>>;

/**
 * The form filled in from a JD. A field is filled when it is empty, or when
 * it still holds what the JD dropped before put there (`before`): another JD
 * replaces it, and leaves out what it does not say. What was typed stays.
 */
export function fillFromJd(form: JobFormState, d: Schemas["JdDetails"], before: JdFilled = {}): { form: JobFormState; filled: JdFilled } {
  const changes: Record<string, string> = {};
  const filled: JdFilled = {};
  const read: Record<keyof JdFilled, string> = {
    title: d.title ?? "", location: d.location ?? "", country: d.country ?? "", workType: d.workType ?? "",
    workArrangement: d.workArrangement ?? "", description: d.description, openings: d.openings ? String(d.openings) : "",
  };
  for (const key of Object.keys(read) as (keyof JdFilled)[]) {
    const current = form[key];
    const fromLastJd = before[key] !== undefined && current === before[key];
    // A new job's 1 is the default, not something typed.
    const empty = !current.trim() || (key === "openings" && current.trim() === "1");
    const value = read[key];
    if (!empty && !fromLastJd) continue; // typed: kept
    if (value) {
      changes[key] = value;
      filled[key] = value;
    } else if (fromLastJd) {
      changes[key] = key === "openings" ? "1" : ""; // the last JD's, which this one does not say
    }
  }
  // The JD's work type and arrangement are the form's own values (or "").
  return { form: { ...form, ...(changes as Partial<JobFormState>) }, filled };
}

/** A company's name without its legal ending and punctuation, to match a JD's to one on record. */
export function companyKey(name: string): string {
  return name.toLowerCase()
    .replace(/\b(sdn\.?\s*bhd\.?|berhad|bhd\.?|pte\.?\s*ltd\.?|ltd\.?|limited|inc\.?|plc|llc|corporation|corp\.?)\s*$/u, "")
    .replace(/\([^)]*\)/gu, " ").replace(/[^\p{L}\p{N}]+/gu, " ").trim();
}

