// Words and numbers for Settings › AI usage: what each AI feature is called,
// money exactly as the API sends it (decimal strings, never floats), token
// counts, month names and how close the month is to its limit.
import type { Schemas } from "../../api/client";

export type Usage = Schemas["AiUsage"];
export type Feature = Schemas["AiFeature"];
export type FeatureUse = Schemas["AiUsageFeature"];
export type Limit = Schemas["AiUsageLimit"];

export const FEATURE: Record<Feature, string> = {
  assistant: "AI assistant",
  assistant_email: "AI email writing",
  fit_score: "Fit scores",
  talent_search: "AI candidate search",
  cv_reading: "CV reading",
  cv_studio: "Formatted CVs",
  jd_blind: "Blind job ads",
  jd_reading: "Job description reading",
  industry_pick: "Industry picks",
  interview_prep: "Interview prep",
  provider_check: "AI provider checks",
};

/** "1234567.5000" -> "1,234,567.5000": only the whole part is grouped, nothing is rounded. */
export function groupDigits(amount: string): string {
  const [whole = "0", fraction] = amount.split(".");
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return fraction === undefined ? grouped : `${grouped}.${fraction}`;
}

/** "USD 1,234.5000", exactly as the API gave the amount. */
export function aiMoney(amount: string, currency: string): string {
  return `${currency} ${groupDigits(amount)}`;
}

/** A cost, or a dash when the prices are not set. */
export function costText(cost: string | null, currency: string): string {
  return cost === null ? "—" : aiMoney(cost, currency);
}

/** An amount as whole ten-thousandths ("3.84" -> 38400), for drawing only. */
export function tenThousandths(amount: string): number {
  const [whole = "0", fraction = ""] = amount.split(".");
  return Number(whole) * 10_000 + Number((fraction + "0000").slice(0, 4));
}

/** Whole ten-thousandths as an amount without trailing zeros (50000 -> "5", 25000 -> "2.5"). */
export function fromTenThousandths(n: number): string {
  const whole = String(Math.floor(n / 10_000));
  const fraction = String(n % 10_000).padStart(4, "0").replace(/0+$/, "");
  return fraction ? `${whole}.${fraction}` : whole;
}

const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 2 });
const whole = new Intl.NumberFormat("en-GB");

/** "1.21M", "45.3K", "812". */
export function shortTokens(n: number): string {
  return compact.format(n);
}

/** "1,210,442". */
export function allTokens(n: number): string {
  return whole.format(n);
}

function monthDate(month: string): Date {
  const [y = "1970", m = "1"] = month.split("-");
  return new Date(Date.UTC(Number(y), Number(m) - 1, 1));
}

/** "2026-11" -> "November 2026". */
export function monthName(month: string): string {
  return monthDate(month).toLocaleDateString("en-GB", { month: "long", year: "numeric", timeZone: "UTC" });
}

/** "2026-11" -> "Nov". */
export function monthShort(month: string): string {
  return monthDate(month).toLocaleDateString("en-US", { month: "short", timeZone: "UTC" }); // "Sep", not "Sept"
}

export type Tone = "ok" | "warn" | "stop";

/** How close this month is to its limit. */
export function limitState(limit: Pick<Limit, "reached" | "usedPercent" | "warnPercent">): { label: string; tone: Tone } {
  if (limit.reached) return { label: "Limit reached", tone: "stop" };
  if (limit.usedPercent >= limit.warnPercent) return { label: "Near the limit", tone: "warn" };
  return { label: "Within limit", tone: "ok" };
}
