// What a parsed CV suggests for a candidate's profile, field by field, and the
// update that applies the suggestions a recruiter ticked. Nothing a person
// entered is replaced unless they tick it.
import type { Candidate, Schemas } from "../api/client";
import { splitName } from "../import/mapping";
import { countryName, formatNotice } from "../labels";
import { formFromCandidate, toUpdateBody } from "./candidateFormState";

type Profile = Schemas["ParsedProfile"];
type Sourced = Schemas["SourcedText"];
type Notice = Schemas["NoticePeriod"];

export type SuggestionKey = "name" | "email" | "phone" | "location" | "nationality" | "residenceCountry" | "industry" | "noticePeriod"
  | "currentTitle" | "currentCompany";

export interface Suggestion {
  key: SuggestionKey;
  label: string;
  value: string; // as the profile will show it
  current: string | null; // what the profile has now
  quote: string; // the CV text it came from
  checked: boolean; // ticked by default only when the profile has nothing there
}

/** "1 month" -> { value: 1, unit: "month" }. */
export function noticeFrom(text: string): Notice | null {
  const m = /^(\d{1,3}) (day|week|month)$/.exec(text);
  return m ? { value: Number(m[1]), unit: m[2] as Notice["unit"] } : null;
}

/** The job the candidate has now (marked current), else the first one listed. */
export function latestJob(p: Profile): Profile["experience"][number] | undefined {
  return p.experience.find((j) => j.current) ?? p.experience[0];
}

const same = (a: string | null, b: string) => (a ?? "").trim().toLowerCase() === b.trim().toLowerCase();

export function suggestions(p: Profile, c: Candidate): Suggestion[] {
  const out: Suggestion[] = [];
  const add = (key: SuggestionKey, label: string, s: Sourced | null | undefined, current: string | null, shown?: string) => {
    if (!s) return;
    const value = shown ?? s.value;
    if (same(current, value)) return; // already on the profile
    out.push({ key, label, value, current: current || null, quote: s.quote, checked: !current });
  };
  add("name", "Name", p.fullName, c.fullName);
  add("email", "Email", p.email, c.email);
  add("phone", "Phone", p.phone, c.phone);
  add("location", "Location", p.location, c.location);
  if (p.nationality) add("nationality", "Nationality", p.nationality, c.nationality && countryName(c.nationality), countryName(p.nationality.value));
  if (p.residenceCountry) {
    add("residenceCountry", "Country of residence", p.residenceCountry, c.residenceCountry && countryName(c.residenceCountry), countryName(p.residenceCountry.value));
  }
  add("industry", "Industry (inferred from the latest employer)", p.industry, c.industry);
  const notice = p.noticePeriod && noticeFrom(p.noticePeriod.value);
  if (notice) add("noticePeriod", "Notice period", p.noticePeriod, c.noticePeriod && formatNotice(c.noticePeriod), formatNotice(notice));
  const job = latestJob(p);
  add("currentTitle", "Current title", job?.title, c.currentTitle);
  add("currentCompany", "Current company", job?.company, c.currentCompany);
  return out;
}

/** Skills in the CV that the profile does not have yet. */
export function newSkills(p: Profile, c: Candidate): Sourced[] {
  const have = new Set(c.skills.map((s) => s.toLowerCase()));
  return p.skills.filter((s) => !have.has(s.value.toLowerCase()));
}

/** The profile update for the ticked suggestions and skills (sent with the version it was read at). */
export function applyBody(p: Profile, c: Candidate, ticked: Set<SuggestionKey>, skills: string[]): Schemas["CandidateUpdate"] {
  const body = toUpdateBody(formFromCandidate(c), c.version);
  const job = latestJob(p);
  if (ticked.has("name") && p.fullName) {
    const [first, last] = splitName(p.fullName.value);
    body.firstName = first;
    body.lastName = last;
  }
  if (ticked.has("email") && p.email) body.email = p.email.value;
  if (ticked.has("phone") && p.phone) body.phone = p.phone.value;
  if (ticked.has("location") && p.location) body.location = p.location.value;
  if (ticked.has("nationality") && p.nationality) body.nationality = p.nationality.value;
  if (ticked.has("residenceCountry") && p.residenceCountry) body.residenceCountry = p.residenceCountry.value;
  if (ticked.has("industry") && p.industry) body.industry = p.industry.value;
  if (ticked.has("noticePeriod") && p.noticePeriod) body.noticePeriod = noticeFrom(p.noticePeriod.value);
  if (ticked.has("currentTitle") && job?.title) body.currentTitle = job.title.value;
  if (ticked.has("currentCompany") && job?.company) body.currentCompany = job.company.value;
  if (skills.length > 0) body.skills = [...c.skills, ...skills].slice(0, 100);
  return body;
}

/** The CV's work history and education, as the profile stores them. */
export function historyFromProfile(p: Profile): Pick<Schemas["CandidateHistoryUpdate"], "experience" | "education"> {
  return {
    experience: p.experience.slice(0, 50).map((j) => ({
      title: j.title?.value ?? null,
      company: j.company?.value ?? null,
      startDate: j.startDate ?? null,
      endDate: j.current ? null : (j.endDate ?? null),
      current: j.current,
      description: null,
    })),
    education: p.education.slice(0, 20).map((e) => ({
      qualification: e.qualification?.value ?? null,
      institution: e.institution?.value ?? null,
      endYear: e.endYear ?? null,
    })),
  };
}
