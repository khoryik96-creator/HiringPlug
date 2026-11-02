// Sends imported rows to the API, a few at a time, through the same endpoints
// as the forms (so every check applies), and reports each row's outcome.
import { api, ApiError, unwrap } from "../api/client";
import { fieldErrors } from "../components/forms";
import { candidateBody, companyBody, contactBody, type ImportKind, rowValues } from "./mapping";

export interface RowResult {
  /** 1-based line in the file (the header is line 1). */
  line: number;
  label: string;
  status: "added" | "exists" | "fix" | "failed";
  message: string;
  link?: string;
}

const CONCURRENCY = 4;

/** "Nestlé  Malaysia" -> "nestle malaysia", to match company names. */
export function nameKey(name: string): string {
  return name.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase().replace(/\s+/g, " ").trim();
}

function outcome(error: unknown, kind: ImportKind): Pick<RowResult, "status" | "message" | "link"> {
  if (error instanceof ApiError) {
    const existing = error.details.existingCandidateId ?? error.details.existingCompanyId ?? error.details.existingContactId;
    if (error.code.endsWith("_DUPLICATE_EMAIL") || error.code.endsWith("_DUPLICATE_NAME")) {
      return { status: "exists", message: error.message, ...(typeof existing === "string" ? { link: `#/${kind}/${existing}` } : {}) };
    }
    const fields = Object.entries(fieldErrors(error));
    if (fields.length > 0) return { status: "fix", message: fields.map(([f, m]) => `${f}: ${m}`).join("; ") };
    return { status: "failed", message: `${error.message} (reference ${error.requestId})` };
  }
  return { status: "failed", message: error instanceof Error ? error.message : "Something went wrong." };
}

/**
 * Imports rows (without the header). For contacts, companies are found by
 * name, or added first when createCompanies is set.
 */
export async function runImport(
  kind: ImportKind,
  rows: string[][],
  mapping: string[],
  options: { createCompanies: boolean },
  onProgress: (done: number) => void,
): Promise<RowResult[]> {
  const companies = new Map<string, Promise<string | null>>();
  const companyId = (name: string) => {
    const key = nameKey(name);
    let found = companies.get(key);
    if (!found) {
      found = (async () => {
        const page = unwrap(await api.GET("/v1/companies", { params: { query: { q: name, limit: 25 } } }));
        const match = page.items.find((c) => nameKey(c.name) === key);
        if (match) return match.id;
        if (!options.createCompanies) return null;
        try {
          return unwrap(await api.POST("/v1/companies", { body: { name } })).id;
        } catch (e) {
          const existing = e instanceof ApiError ? e.details.existingCompanyId : undefined;
          if (typeof existing === "string") return existing;
          throw e;
        }
      })();
      companies.set(key, found);
    }
    return found;
  };

  const importRow = async (row: string[], index: number): Promise<RowResult> => {
    const v = rowValues(row, mapping);
    const line = index + 2;
    const label = v.name ?? v.fullName ?? ([v.firstName, v.lastName].filter(Boolean).join(" ") || `Line ${String(line)}`);
    try {
      if (kind === "candidates") {
        const c = unwrap(await api.POST("/v1/candidates", { body: candidateBody(v) }));
        return { line, label, status: "added", message: "", link: `#/candidates/${c.id}` };
      }
      if (kind === "companies") {
        const c = unwrap(await api.POST("/v1/companies", { body: companyBody(v) }));
        return { line, label, status: "added", message: "", link: `#/companies/${c.id}` };
      }
      if (!v.company) return { line, label, status: "fix", message: "company: is empty" };
      const id = await companyId(v.company);
      if (!id) return { line, label, status: "fix", message: `company: no company named "${v.company}" (add it, or tick "add missing companies")` };
      const c = unwrap(await api.POST("/v1/contacts", { body: contactBody(v, id) }));
      return { line, label, status: "added", message: "", link: `#/contacts/${c.id}` };
    } catch (e) {
      return { line, label, ...outcome(e, kind) };
    }
  };

  const results: RowResult[] = new Array<RowResult>(rows.length);
  let next = 0;
  let done = 0;
  const worker = async () => {
    while (next < rows.length) {
      const i = next++;
      results[i] = await importRow(rows[i] ?? [], i);
      onProgress(++done);
    }
  };
  await Promise.all(Array.from({ length: Math.min(CONCURRENCY, rows.length) }, worker));
  return results;
}
