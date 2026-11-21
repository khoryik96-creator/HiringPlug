const STATUS = new Set(["verified", "unverified", "catch_all", "unavailable", "user_managed"]);

const normalized = (text) => (typeof text === "string" ? text.trim().replace(/\s+/g, " ").toLowerCase() : "");
// Apollo returns LinkedIn links as http://www.linkedin.com/...; saved HR links
// are https. Compare the public handle only, never the scheme or host form.
const linkedinHandle = (value, kind) => {
  const m = /^(?:https?:\/\/)?(?:www\.|[a-z]{2}\.)?linkedin\.com\/(in|company)\/([A-Za-z0-9_%~-]+)\/?$/i.exec(
    String(value || "").trim(),
  );
  return m && m[1].toLowerCase() === kind ? m[2].toLowerCase() : "";
};
const LEGAL = new Set([
  "pt",
  "tbk",
  "sdn",
  "bhd",
  "berhad",
  "pte",
  "ltd",
  "limited",
  "inc",
  "llc",
  "corp",
  "corporation",
  "co",
  "plc",
  "gmbh",
  "the",
]);
const nameWords = (text) => {
  const words = normalized(text)
    .normalize("NFKC")
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim()
    .split(/\s+/);
  // Legal affixes may wrap a brand, but words inside it remain significant.
  while (words.length && LEGAL.has(words[0])) words.shift();
  while (words.length && LEGAL.has(words.at(-1))) words.pop();
  return words.join(" ");
};
function sameCompanyName(a, b) {
  const first = nameWords(a),
    second = nameWords(b);
  return !!first && first === second;
}
function companyMatches(org, expected) {
  const company = linkedinHandle(org?.linkedin_url, "company");
  if (company) {
    return /^[0-9]+$/.test(company)
      ? company === expected.company_id
      : company === linkedinHandle(expected.company_url, "company");
  }
  return false;
}
const object = (value) => value && typeof value === "object" && !Array.isArray(value);
// Exact mailbox domains only: sharing a provider's brand prefix is not evidence.
const PERSONAL_MAIL_DOMAINS = new Set([
  "gmail.com",
  "googlemail.com",
  "yahoo.com",
  "yahoo.com.my",
  "yahoo.com.sg",
  "yahoo.co.uk",
  "ymail.com",
  "rocketmail.com",
  "hotmail.com",
  "hotmail.co.uk",
  "outlook.com",
  "live.com",
  "msn.com",
  "aol.com",
  "icloud.com",
  "me.com",
  "mac.com",
  "mail.com",
  "email.com",
  "gmx.com",
  "gmx.net",
  "proton.me",
  "protonmail.com",
  "protonmail.ch",
  "pm.me",
]);
function emailCandidate(value) {
  if (value === null || value === undefined || value === "") return { reason: "email_not_returned" };
  if (
    typeof value !== "string" ||
    value.length > 254 ||
    !/^[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,63}$/.test(value)
  )
    return { reason: "email_unusable" };
  if (value.toLowerCase() === "email_not_unlocked@domain.com") return { reason: "email_redacted" };
  if (PERSONAL_MAIL_DOMAINS.has(value.split("@")[1].toLowerCase())) return { reason: "personal_email_only" };
  return { email: value };
}
export function emailResult(data, expected) {
  if (!object(data) || !Object.hasOwn(data, "person")) return { state: "unconfirmed" };
  const person = data.person;
  if (person === null) return { state: "not_found", reason: "profile_not_found" };
  if (!object(person)) return { state: "unconfirmed" };
  const profile = linkedinHandle(person.linkedin_url, "in");
  if (
    !profile ||
    profile !== linkedinHandle(expected.url, "in") ||
    !companyMatches(person.organization, expected) ||
    [data.match_confidence, person.match_confidence].some((value) => value === "low" || value === "none")
  )
    return { state: "mismatch" };
  const primary = emailCandidate(person.email);
  if (primary.email)
    return {
      state: "found",
      email: primary.email,
      email_status: STATUS.has(person.email_status) ? person.email_status : "unknown",
    };
  // Team-saved contacts can contain the business address while the public
  // person.email field is empty. Never borrow a separate contact's email:
  // require a positive person-ID/profile binding and reject contradictory
  // profile/company metadata, even when the parent person matched.
  const contact = person.contact;
  if (object(contact)) {
    const contactProfile = linkedinHandle(contact.linkedin_url, "in");
    const idBound = typeof person.id === "string" && !!person.id && contact.person_id === person.id;
    const profileBound = !!contactProfile && contactProfile === profile;
    const organizationId = person.organization_id || person.organization?.id;
    const sameOrganization =
      typeof organizationId === "string" && !!organizationId && contact.organization_id === organizationId;
    const employerBound =
      sameOrganization || (contact.organization != null && companyMatches(contact.organization, expected));
    const conflict =
      (contact.person_id != null && contact.person_id !== person.id) ||
      (contact.linkedin_url != null && !profileBound) ||
      (contact.organization != null && !companyMatches(contact.organization, expected)) ||
      (contact.organization_name != null && !sameCompanyName(contact.organization_name, expected.company)) ||
      (contact.organization_id != null && organizationId != null && !sameOrganization);
    if (conflict || !employerBound || (!idBound && !profileBound))
      return { state: "not_found", reason: "contact_unconfirmed" };
    const saved = emailCandidate(contact.email);
    if (saved.email)
      return {
        state: "found",
        email: saved.email,
        email_status: STATUS.has(contact.email_status) ? contact.email_status : "unknown",
        email_source: "saved_contact",
      };
    if (saved.reason !== "email_not_returned") return { state: "not_found", reason: saved.reason };
  }
  return { state: "not_found", reason: primary.reason };
}
