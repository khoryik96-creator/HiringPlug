// A new contact from their LinkedIn profile: what the companion read (name,
// current roles), the companies in HiringPlug that may be theirs, and what
// happens once the contact is saved (an email guessed from colleagues', then
// verified; their time at the company checked on LinkedIn, which records
// since when, or marks them as having left).
import { api, type Contact, type Schemas, unwrap } from "../api/client";
import { companionCall, operationId } from "../companion";
import { companyPage } from "./linkedinCompany";

export interface Role { company_url: string; company_name: string; title: string; since: string }
export interface ProfileRead { url: string; name: string; roles: Role[] }

/** Their LinkedIn profile as the companion opens it, or "" if it is not one. */
export function profilePage(raw: string): string {
  const s = raw.trim();
  if (!s) return "";
  try {
    const u = new URL(/^https?:\/\//i.test(s) ? s : `https://${s}`);
    const m = /^\/in\/([A-Za-z0-9_%~-]+)\/?/.exec(u.pathname);
    return /(^|\.)linkedin\.com$/i.test(u.hostname) && m ? `https://www.linkedin.com/in/${m[1] ?? ""}` : "";
  } catch {
    return "";
  }
}

export function readProfile(url: string): Promise<ProfileRead> {
  return companionCall<ProfileRead>("profile", { operation_id: operationId(), url });
}

/** The current role at the company chosen (its LinkedIn page), else their first current role. */
export function roleAt(roles: Role[], companyLinkedin: string | null | undefined): Role | undefined {
  const page = companyPage(companyLinkedin ?? "");
  return roles.find((r) => page && r.company_url === page) ?? roles[0];
}

export interface CompanyMatch { id: string; name: string; detail: string | null; samePage: boolean }

/** Companies in HiringPlug that may be where they work: the same LinkedIn page first, then similar names. */
export function companiesFor(role: Role): Promise<CompanyMatch[]> {
  return companiesNamed(role.company_name, role.company_url);
}

const SUFFIX = /\b(sdn\.?|bhd\.?|berhad|plc|inc\.?|ltd\.?|limited|llc|pte\.?|corp\.?)\b|\(m\)/gi;
/** A company's name without legal suffixes, case or punctuation ("Maju Bank (M) Berhad" -> "maju bank"). */
export const plainName = (name: string) => name.toLowerCase().replace(SUFFIX, " ").replace(/[^\p{L}\p{N}]+/gu, " ").trim();

/**
 * Companies in HiringPlug that may be the one named (or with this LinkedIn
 * page): the same page first, then the same name (suffixes aside), then
 * similar names.
 */
export async function companiesNamed(name: string, page = ""): Promise<CompanyMatch[]> {
  // The first words of the name find "Maju Bank Berhad" from "Maju Bank".
  const q = name.replace(SUFFIX, " ").replace(/[^\p{L}\p{N}&' -]+/gu, " ").trim().split(/\s+/).slice(0, 2).join(" ");
  // More than are offered: the one with exactly this name is offered even when many share its first words.
  const items = q ? unwrap(await api.GET("/v1/companies", { params: { query: { q, limit: 25 } } })).items : [];
  const plain = plainName(name);
  const rank = (c: { name: string; linkedinUrl: string | null }) =>
    (page && companyPage(c.linkedinUrl ?? "") === page ? 2 : 0) + (plainName(c.name) === plain ? 1 : 0);
  return [...items]
    .sort((a, b) => rank(b) - rank(a))
    .slice(0, 8)
    .map((c) => ({ id: c.id, name: c.name, detail: c.city, samePage: !!page && companyPage(c.linkedinUrl ?? "") === page }));
}

/** "Farah binti Aziz" -> first "Farah binti", last "Aziz". */
export function nameParts(full: string): { firstName: string; lastName: string } {
  const words = full.trim().split(/\s+/);
  return words.length < 2 ? { firstName: words[0] ?? "", lastName: "" } : { firstName: words.slice(0, -1).join(" "), lastName: words.at(-1) ?? "" };
}

/**
 * Once a new contact is saved: with no email, one guessed from colleagues'
 * (saving it verifies it); with a LinkedIn profile at a company with a
 * LinkedIn page, and the companion in this browser, their time there checked
 * (since when, or left).
 */
export async function afterContactAdded(contact: Contact, withCompanion: boolean): Promise<void> {
  if (!contact.email) {
    const guess = unwrap(await api.GET("/v1/contacts/{contactId}/email-suggestion", { params: { path: { contactId: contact.id } } })).suggestion;
    if (guess) await api.PATCH("/v1/contacts/{contactId}", { params: { path: { contactId: contact.id } }, body: { version: contact.version, email: guess.email } });
  }
  if (!contact.linkedinUrl || !withCompanion) return; // the check needs the companion in this browser
  const started = unwrap(await api.POST("/v1/contacts/linkedin-checks", { body: { contactId: contact.id } }));
  const check = started.checks[0];
  if (!check) return; // the company has no LinkedIn page
  let body: Schemas["ContactLinkedinCheckAnswer"];
  try {
    const company = await companionCall<{ company_id: string; company_name?: string }>("companyId", { operation_id: operationId(), company_url: started.companyUrl });
    const read = await companionCall<{ url: string; company_url: string; company_id: string; joined: string; left: string; current: boolean | null; matched_by_name?: boolean }>("employment",
      { operation_id: check.id, url: check.profileUrl, company_url: started.companyUrl, company_id: company.company_id,
        company_names: [started.companyName, company.company_name ?? ""].filter(Boolean) });
    body = { profileUrl: read.url, companyUrl: read.company_url, linkedinCompanyId: read.company_id, joined: read.joined, left: read.left, current: read.current,
      ...(read.matched_by_name ? { matchedByName: true } : {}) };
  } catch (e) {
    body = { error: e instanceof Error ? e.message.slice(0, 300) : "Could not be read." };
  }
  await api.POST("/v1/contacts/linkedin-checks/{checkId}/answer", { params: { path: { checkId: check.id } }, body });
}
