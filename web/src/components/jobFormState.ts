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
export function fillFromJd(form: JobFormState, d: Schemas["JdDetails"]): JobFormState {
  const pick = (current: string, read: string | null | undefined) => current.trim() ? current : read ?? current;
  return {
    ...form,
    title: pick(form.title, d.title),
    location: pick(form.location, d.location),
    country: pick(form.country, d.country),
    workType: form.workType || (d.workType ?? ""),
    workArrangement: form.workArrangement || (d.workArrangement ?? ""),
    description: pick(form.description, d.description),
  };
}

