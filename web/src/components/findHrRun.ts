// Find HR's run: the companion reads the scan's next page in the recruiter's
// browser and the page is saved, until the scan is done. A page that fails
// is tried again on its own (after 5, 15 and 30 seconds); then the scan stops
// at that page, to resume there. Sign-in and similar stops are not retried.
import { api, type Schemas, unwrap } from "../api/client";
import { companionCall, operationId } from "../companion";

type Scan = Schemas["HrScan"];
interface Page { people: { url: string; name: string; title: string; location: string }[]; has_next: boolean }

export const RETRY_WAITS = [5_000, 15_000, 30_000];
/** A stop a retry cannot get past: the recruiter has to act. */
const NEEDS_YOU = /sign in|stopped|busy|update|connect|company filter|location filter/i;

export interface Run {
  operation: string;
  stopped: boolean; // set by stopRun while a read is in progress (read through isStopped)
  /** "Retrying page 2 in 15s (LinkedIn did not finish loading)". */
  onRetry: (message: string) => void;
  wait: (ms: number) => Promise<void>;
}

export function newRun(onRetry: (m: string) => void, wait = (ms: number) => new Promise<void>((r) => { setTimeout(r, ms); })): Run {
  return { operation: operationId(), stopped: false, onRetry, wait };
}

async function savePage(companyId: string, body: Schemas["HrScanPage"]): Promise<Scan> {
  return unwrap(await api.POST("/v1/companies/{companyId}/hr-scan/pages", { params: { path: { companyId } }, body }));
}

const isStopped = (run: Run) => run.stopped;

/** Reads the scan's pages from where it is (a stopped scan resumes at its failed page), calling onScan after each. */
export async function runScan(scan: Scan, run: Run, onScan: (s: Scan) => void): Promise<Scan> {
  let s: Scan = scan.status === "stopped" ? { ...scan, status: "running" } : scan; // Resume
  while (s.status === "running" && s.next && !isStopped(run)) {
    const { query, page } = s.next;
    let read: Page | null = null;
    let failure = "";
    for (let attempt = 0; attempt <= RETRY_WAITS.length && !isStopped(run); attempt++) {
      try {
        read = await companionCall<Page>("peoplePage", {
          operation_id: run.operation, company_id: s.linkedinCompanyId, company_name: s.linkedinCompanyName,
          ...(s.geoId && s.countryName ? { geo_id: s.geoId, country: s.countryName } : {}),
          query: s.queries[query], keywords: s.keywords.join(","), page,
        }, 90_000);
        break;
      } catch (e) {
        failure = e instanceof Error ? e.message : String(e);
        const waitMs = RETRY_WAITS[attempt];
        if (isStopped(run) || NEEDS_YOU.test(failure) || waitMs === undefined) break;
        run.onRetry(`Page ${String(page)} did not load (${failure}). Trying again in ${String(waitMs / 1000)}s…`);
        await run.wait(waitMs);
      }
    }
    if (isStopped(run)) break;
    s = read
      ? await savePage(s.companyId, { query, page, people: read.people, hasNext: read.has_next })
      : await savePage(s.companyId, { query, page, error: failure.slice(0, 300) || "The page could not be read." });
    onScan(s);
    run.onRetry("");
  }
  return s;
}

/** Stops the read in progress (the companion closes its tab). */
export function stopRun(run: Run) {
  run.stopped = true;
  void companionCall("cancel", { operation_id: run.operation }, 5_000).catch(() => undefined);
}

/** "Farah binti Aziz" -> first "Farah binti", last "Aziz" (one word: first only). */
export function splitName(full: string): { firstName: string; lastName: string } {
  const words = full.trim().split(/\s+/);
  if (words.length < 2) return { firstName: words[0] ?? "", lastName: "" };
  return { firstName: words.slice(0, -1).join(" "), lastName: words.at(-1) ?? "" };
}
