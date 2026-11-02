// The candidate edit form's state, its checks, and its conversion to the API request.
import type { Candidate, Schemas } from "../api/client";
import { orNull } from "./forms";

export interface CandidateFormState {
  firstName: string;
  lastName: string;
  email: string;
  phone: string;
  nationality: string;
  waStatus: "" | Schemas["WorkAuthorizationStatus"];
  waBasis: "" | Schemas["WorkAuthorizationBasis"];
  waNote: string;
  noticeValue: string;
  noticeUnit: Schemas["NoticePeriodUnit"];
  industry: string;
  specialization: string;
  currentTitle: string;
  currentCompany: string;
  location: string;
  source: "" | Schemas["CandidateSource"];
  tags: string[];
  skills: string[];
}

/** Where the API reports a problem with each input (the VALIDATION_FAILED field path). */
export const FIELD_PATHS: Record<keyof CandidateFormState, string> = {
  firstName: "firstName",
  lastName: "lastName",
  email: "email",
  phone: "phone",
  nationality: "nationality",
  waStatus: "workAuthorization.status",
  waBasis: "workAuthorization.basis",
  waNote: "workAuthorization.note",
  noticeValue: "noticePeriod.value",
  noticeUnit: "noticePeriod.unit",
  industry: "industry",
  specialization: "specialization",
  currentTitle: "currentTitle",
  currentCompany: "currentCompany",
  location: "location",
  source: "source",
  tags: "tags",
  skills: "skills",
};

export function formFromCandidate(c: Candidate): CandidateFormState {
  return {
    firstName: c.firstName,
    lastName: c.lastName,
    email: c.email ?? "",
    phone: c.phone ?? "",
    nationality: c.nationality ?? "",
    waStatus: c.workAuthorization?.status ?? "",
    waBasis: c.workAuthorization?.basis ?? "",
    waNote: c.workAuthorization?.note ?? "",
    noticeValue: c.noticePeriod ? String(c.noticePeriod.value) : "",
    noticeUnit: c.noticePeriod?.unit ?? "month",
    industry: c.industry ?? "",
    specialization: c.specialization ?? "",
    currentTitle: c.currentTitle ?? "",
    currentCompany: c.currentCompany ?? "",
    location: c.location ?? "",
    source: c.source ?? "",
    tags: c.tags,
    skills: c.skills,
  };
}

/**
 * Mistakes worth catching before sending, keyed like the API's field errors.
 * The API checks everything again; this only gives clearer messages.
 */
export function candidateProblems(f: CandidateFormState): Record<string, string> {
  const problems: Record<string, string> = {};
  const nationality = f.nationality.trim();
  if (nationality !== "" && !/^[A-Za-z]{2}$/.test(nationality)) {
    problems[FIELD_PATHS.nationality] = "Use the 2-letter country code, e.g. MY for Malaysia.";
  }
  return problems;
}

/** The PATCH body: every field is sent; empty inputs clear the field. */
export function toUpdateBody(f: CandidateFormState, version: number): Schemas["CandidateUpdate"] {
  return {
    version,
    firstName: f.firstName,
    lastName: f.lastName,
    email: orNull(f.email),
    phone: orNull(f.phone),
    nationality: orNull(f.nationality.toUpperCase()),
    workAuthorization: f.waStatus
      ? {
          status: f.waStatus,
          basis: f.waStatus === "expat_eligible" && f.waBasis ? f.waBasis : null,
          note: orNull(f.waNote),
        }
      : null,
    noticePeriod: f.noticeValue.trim() === "" ? null : { value: Number(f.noticeValue), unit: f.noticeUnit },
    industry: orNull(f.industry),
    specialization: orNull(f.specialization),
    currentTitle: orNull(f.currentTitle),
    currentCompany: orNull(f.currentCompany),
    location: orNull(f.location),
    source: f.source === "" ? null : f.source,
    tags: f.tags,
    skills: f.skills,
  };
}
