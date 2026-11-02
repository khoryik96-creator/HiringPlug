// Display names for the contract's enums, used both for form options and for
// read-only text, so the two always say the same thing. Each map is a Record
// over the generated enum type: a value added to the contract does not compile
// until it has a label here, and then every select offers it.
import type { Schemas } from "./api/client";

export const WORK_AUTH_STATUS: Record<Schemas["WorkAuthorizationStatus"], string> = {
  local: "Local",
  expat_eligible: "Expat, can work here",
  requires_visa: "Requires a visa",
};

export const WORK_AUTH_BASIS: Record<Schemas["WorkAuthorizationBasis"], string> = {
  rp_t: "Residence Pass-Talent (RP-T)",
  permanent_resident: "Permanent resident",
  spouse_visa: "Spouse visa",
  other: "Other pass",
};

export const CANDIDATE_SOURCE: Record<Schemas["CandidateSource"], string> = {
  jobstreet: "JobStreet",
  linkedin: "LinkedIn",
  referral: "Referral",
  careers_page: "Careers page",
  direct: "Direct",
  other: "Other",
};

export const NOTICE_UNIT: Record<Schemas["NoticePeriodUnit"], { one: string; many: string }> = {
  day: { one: "day", many: "days" },
  week: { one: "week", many: "weeks" },
  month: { one: "month", many: "months" },
};

export const PAY_PERIOD: Record<Schemas["PayPeriod"], string> = {
  month: "month",
  year: "year",
  day: "day",
  hour: "hour",
};

export const COMPANY_TYPE: Record<Schemas["CompanyType"], string> = {
  client: "Client",
  prospect: "Prospect",
  internal: "Our organisation",
};

export const COMPANY_SIZE: Record<Schemas["CompanySize"], string> = {
  micro: "1–10 people",
  small: "11–50 people",
  medium: "51–200 people",
  large: "201–1,000 people",
  enterprise: "Over 1,000 people",
};

/** The values of a label map, in its order: the options of a select. */
export function keysOf<K extends string>(labels: Record<K, unknown>): K[] {
  return Object.keys(labels) as K[];
}

/** "Immediate", "1 month", "3 weeks", or "—" when not recorded. */
export function formatNotice(notice: Schemas["NoticePeriod"] | null | undefined): string {
  if (!notice) return "—";
  if (notice.value === 0) return "Immediate";
  const unit = NOTICE_UNIT[notice.unit];
  return `${notice.value} ${notice.value === 1 ? unit.one : unit.many}`;
}

const regions = new Intl.DisplayNames(["en"], { type: "region", fallback: "none" });

/** "MY" -> "Malaysia"; undefined for codes that are not countries. */
export function regionName(code: string): string | undefined {
  try {
    return regions.of(code);
  } catch {
    return undefined; // not even shaped like a code
  }
}

/** "Malaysia (MY)", so a mistyped code is easy to spot; just the code if it is unknown. */
export function countryName(code: string): string {
  const name = regionName(code);
  return name ? `${name} (${code})` : code;
}
