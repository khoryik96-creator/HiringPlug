// A company's LinkedIn page, read by the LinkedIn companion in the
// recruiter's own browser (extension/linkedin-companion/company.js).
import type { Schemas } from "../api/client";

/** What the companion reads from a company's About page. */
export interface LinkedinAbout {
  company_url: string;
  name: string;
  industry: string;
  size: string; // as LinkedIn says it: "51-200 employees"
  website: string;
  headquarters: string;
  about: string;
}

/** The company's LinkedIn page as the companion opens it, or "" if it is not one. */
export function companyPage(raw: string): string {
  const s = raw.trim();
  if (!s) return "";
  try {
    const u = new URL(/^https?:\/\//i.test(s) ? s : `https://${s}`);
    const m = /^\/company\/([A-Za-z0-9_-]+)(?:\/.*)?$/.exec(u.pathname);
    return /(^|\.)linkedin\.com$/i.test(u.hostname) && m ? `https://www.linkedin.com/company/${m[1] ?? ""}/` : "";
  } catch {
    return "";
  }
}

/** The company's name in its LinkedIn page's address ("contoh-logistik"), or undefined if it is not a company page. */
export function companySlug(raw: string): string | undefined {
  return /\/company\/([A-Za-z0-9_-]+)\//.exec(companyPage(raw))?.[1];
}

/**
 * LinkedIn's size ("2-10 employees", "1,001-5,000 employees", "10,001+
 * employees") as one of ours, by its lower figure; null if it is not one.
 */
export function sizeFromLinkedin(text: string): Schemas["CompanySize"] | null {
  const m = /^\s*([\d,]+)\s*(?:[-–]\s*[\d,]+|\+)?\s*employees?\b/i.exec(text);
  if (!m?.[1]) return null;
  const low = Number(m[1].replace(/,/g, ""));
  if (!Number.isFinite(low) || low < 1) return null;
  return low < 11 ? "micro" : low < 51 ? "small" : low < 201 ? "medium" : low < 1001 ? "large" : "enterprise";
}

/** Whether this companion version reads company pages (1.1.0 and later). */
export function companionCan(version: string | null): boolean {
  const [major = 0, minor = 0] = (version ?? "").split(".").map(Number);
  return major > 1 || (major === 1 && minor >= 1);
}
