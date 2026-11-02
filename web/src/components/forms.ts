// Small helpers shared by the edit forms.
import { ApiError } from "../api/client";

/**
 * Problems found in the browser before anything is sent. They have the same
 * shape as the API's VALIDATION_FAILED details, so both are shown the same way.
 */
export class FormInvalid extends Error {
  constructor(readonly fields: Record<string, string>) {
    super("Some fields need attention.");
    this.name = "FormInvalid";
  }
}

/** API field path -> message (e.g. "workAuthorization.basis"), from VALIDATION_FAILED or FormInvalid. */
export function fieldErrors(error: unknown): Record<string, string> {
  if (error instanceof FormInvalid) return error.fields;
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

/** Whether two request bodies built by the same function are equal, i.e. nothing changed. */
export function sameBody(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

export const COUNTRIES = ["MY", "SG", "ID", "TH", "PH", "VN", "IN", "CN", "HK", "TW", "JP", "KR", "AU", "NZ", "GB", "US"];
export const CURRENCIES = ["MYR", "SGD", "USD", "IDR", "THB", "PHP", "VND", "INR", "CNY", "HKD", "AUD", "GBP", "EUR"];
