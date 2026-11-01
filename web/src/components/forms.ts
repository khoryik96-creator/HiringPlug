// Small helpers shared by the edit forms.
import { ApiError, type Schemas } from "../api/client";

/** Field name -> message, from a VALIDATION_FAILED error (e.g. "workAuthorization.basis"). */
export function fieldErrors(error: unknown): Record<string, string> {
  if (!(error instanceof ApiError) || error.code !== "VALIDATION_FAILED") return {};
  const fields = error.details.fields;
  if (!Array.isArray(fields)) return {};
  const out: Record<string, string> = {};
  for (const f of fields as unknown[]) {
    if (typeof f === "object" && f !== null && "field" in f && "message" in f) {
      out[String(f.field)] = String(f.message);
    }
  }
  return out;
}

/** "" becomes null, so clearing an input clears the field. */
export function orNull(value: string): string | null {
  const v = value.trim();
  return v === "" ? null : v;
}

export const COUNTRIES = ["MY", "SG", "ID", "TH", "PH", "VN", "IN", "CN", "HK", "TW", "JP", "KR", "AU", "NZ", "GB", "US"];
export const CURRENCIES = ["MYR", "SGD", "USD", "IDR", "THB", "PHP", "VND", "INR", "CNY", "HKD", "AUD", "GBP", "EUR"];
export const PERIODS: Schemas["PayPeriod"][] = ["month", "year", "day", "hour"];
export const SOURCES: Schemas["CandidateSource"][] = ["jobstreet", "linkedin", "referral", "careers_page", "direct", "other"];
