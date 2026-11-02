// What can be imported, how spreadsheet columns map to fields, and how one
// row becomes an API request. Values are tidied only where the meaning is
// clear (a country name to its code); everything else goes to the API as
// written and is checked there, like a form.
import { regionName } from "../labels";

export type ImportKind = "candidates" | "companies" | "contacts";

export interface FieldDef {
  key: string;
  label: string;
  required?: boolean;
  /** Lower-case header names that mean this field. */
  headers: string[];
}

const NAME_FIELDS: FieldDef[] = [
  { key: "firstName", label: "First name", headers: ["first name", "firstname", "given name", "first"] },
  { key: "lastName", label: "Last name", headers: ["last name", "lastname", "surname", "family name", "last"] },
  { key: "fullName", label: "Full name (split into first and last)", headers: ["name", "full name", "fullname", "candidate name", "contact name", "candidate"] },
];

export const FIELDS: Record<ImportKind, FieldDef[]> = {
  candidates: [
    ...NAME_FIELDS,
    { key: "email", label: "Email", headers: ["email", "e-mail", "email address"] },
    { key: "phone", label: "Phone", headers: ["phone", "mobile", "mobile number", "contact number", "phone number", "tel", "hp"] },
    { key: "nationality", label: "Nationality", headers: ["nationality", "citizenship", "country"] },
    { key: "currentTitle", label: "Current title", headers: ["current title", "title", "job title", "position", "current position", "designation"] },
    { key: "currentCompany", label: "Current company", headers: ["current company", "company", "employer", "current employer"] },
    { key: "location", label: "Location", headers: ["location", "city", "address", "state"] },
    { key: "industry", label: "Industry", headers: ["industry", "sector"] },
    { key: "specialization", label: "Specialization", headers: ["specialization", "specialisation", "function", "skills area"] },
    { key: "source", label: "Source", headers: ["source", "channel"] },
  ],
  companies: [
    { key: "name", label: "Name", required: true, headers: ["name", "company", "company name", "client", "account", "organisation", "organization"] },
    { key: "type", label: "Type (client / prospect / internal)", headers: ["type", "status", "relationship"] },
    { key: "website", label: "Website", headers: ["website", "web", "url", "domain", "site"] },
    { key: "industry", label: "Industry", headers: ["industry", "sector"] },
    { key: "phone", label: "Phone", headers: ["phone", "tel", "telephone", "office phone"] },
    { key: "address", label: "Address", headers: ["address", "street"] },
    { key: "city", label: "City", headers: ["city", "town", "location"] },
    { key: "country", label: "Country", headers: ["country"] },
    { key: "registrationNumber", label: "Registration no.", headers: ["registration number", "registration no", "ssm", "uen", "reg no", "company number"] },
    { key: "about", label: "About", headers: ["about", "description", "notes"] },
  ],
  contacts: [
    { key: "company", label: "Company name", required: true, headers: ["company", "company name", "client", "organisation", "organization", "account"] },
    ...NAME_FIELDS,
    { key: "title", label: "Job title", headers: ["title", "job title", "position", "designation", "role"] },
    { key: "email", label: "Email", headers: ["email", "e-mail", "email address"] },
    { key: "phone", label: "Office phone", headers: ["phone", "office phone", "tel", "telephone", "work phone"] },
    { key: "mobile", label: "Mobile", headers: ["mobile", "mobile phone", "cell", "hp", "handphone"] },
    { key: "linkedinUrl", label: "LinkedIn", headers: ["linkedin", "linkedin url", "linkedin profile"] },
    { key: "notes", label: "Notes", headers: ["notes", "note", "comments", "remarks"] },
  ],
};

/** Column index -> field key ("" = not imported), guessed from the headers. */
export function autoMap(kind: ImportKind, headers: string[]): string[] {
  const used = new Set<string>();
  return headers.map((h) => {
    const name = h.trim().toLowerCase().replace(/[_*:]+/g, " ").replace(/\s+/g, " ");
    const field = FIELDS[kind].find((f) => !used.has(f.key) && f.headers.includes(name));
    if (!field) return "";
    used.add(field.key);
    return field.key;
  });
}

/** Problems with the mapping itself, before any row is sent. */
export function mappingProblems(kind: ImportKind, mapping: string[]): string[] {
  const mapped = new Set(mapping.filter(Boolean));
  const problems = FIELDS[kind].filter((f) => f.required && !mapped.has(f.key)).map((f) => `Choose the column with ${f.label.toLowerCase()}.`);
  if (kind !== "companies" && !mapped.has("firstName") && !mapped.has("fullName")) {
    problems.push("Choose the column with the first name or the full name.");
  }
  return problems;
}

/** The mapped values of one row, trimmed; empty cells are left out. */
export function rowValues(row: string[], mapping: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  mapping.forEach((key, i) => {
    const v = (row[i] ?? "").trim();
    if (key && v) out[key] = v;
  });
  return out;
}

const DEMONYMS: Record<string, string> = {
  malaysian: "MY", singaporean: "SG", indonesian: "ID", filipino: "PH", filipina: "PH", thai: "TH", vietnamese: "VN",
  indian: "IN", chinese: "CN", japanese: "JP", korean: "KR", "south korean": "KR", australian: "AU", british: "GB",
  american: "US", bangladeshi: "BD", pakistani: "PK", "sri lankan": "LK", nepalese: "NP", burmese: "MM", myanmar: "MM",
  "hong konger": "HK", taiwanese: "TW", "new zealander": "NZ", german: "DE", french: "FR", dutch: "NL", canadian: "CA",
};

let countryNames: Map<string, string> | null = null;

/** "Malaysia", "Malaysian" or "my" -> "MY"; the text unchanged if unknown. */
export function countryCode(text: string): string {
  const t = text.trim();
  if (/^[A-Za-z]{2}$/.test(t)) return t.toUpperCase();
  if (!countryNames) {
    countryNames = new Map();
    const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
    for (const a of letters) for (const b of letters) {
      const name = regionName(a + b);
      if (name) countryNames.set(name.toLowerCase(), a + b);
    }
  }
  const lower = t.toLowerCase();
  return countryNames.get(lower) ?? DEMONYMS[lower] ?? t;
}

/** "Siti Aisyah" -> ["Siti", "Aisyah"]. */
export function splitName(full: string): [string, string] {
  const parts = full.trim().split(/\s+/);
  return [parts[0] ?? "", parts.slice(1).join(" ")];
}

type Source = "jobstreet" | "linkedin" | "referral" | "careers_page" | "direct" | "other";

/** "JobStreet", "Linked In", "Referral" -> the API's source values; anything else is "other". */
export function sourceValue(text: string): Source {
  const t = text.toLowerCase().replace(/[\s_-]+/g, "");
  if (t.includes("jobstreet")) return "jobstreet";
  if (t.includes("linkedin")) return "linkedin";
  if (t.includes("refer")) return "referral";
  if (t.includes("career") || t.includes("website")) return "careers_page";
  if (t.includes("direct") || t.includes("walkin")) return "direct";
  return "other";
}

function names(v: Record<string, string>): { firstName: string; lastName: string } {
  if (v.firstName) return { firstName: v.firstName, lastName: v.lastName ?? "" };
  const [first, last] = splitName(v.fullName ?? "");
  return { firstName: first, lastName: v.lastName ?? last };
}

/** The API request body for one row (companies and contacts may need a company id). */
export function candidateBody(v: Record<string, string>) {
  return {
    ...names(v),
    email: v.email ?? null, phone: v.phone ?? null,
    nationality: v.nationality ? countryCode(v.nationality) : null,
    currentTitle: v.currentTitle ?? null, currentCompany: v.currentCompany ?? null, location: v.location ?? null,
    industry: v.industry ?? null, specialization: v.specialization ?? null,
    source: v.source ? sourceValue(v.source) : null,
  };
}

export function companyBody(v: Record<string, string>) {
  const type = (v.type ?? "").toLowerCase();
  return {
    name: v.name ?? "",
    type: type.startsWith("prospect") || type.startsWith("lead") ? "prospect" as const
      : type.startsWith("internal") || type.startsWith("department") ? "internal" as const : "client" as const,
    website: v.website ?? null, industry: v.industry ?? null, phone: v.phone ?? null, address: v.address ?? null,
    city: v.city ?? null, country: v.country ? countryCode(v.country) : null,
    registrationNumber: v.registrationNumber ?? null, about: v.about ?? null,
  };
}

export function contactBody(v: Record<string, string>, companyId: string) {
  return {
    companyId, ...names(v),
    title: v.title ?? null, email: v.email ?? null, phone: v.phone ?? null, mobile: v.mobile ?? null,
    linkedinUrl: v.linkedinUrl ?? null, notes: v.notes ?? null,
  };
}
