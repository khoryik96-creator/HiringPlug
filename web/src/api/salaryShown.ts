// Answers that show a candidate's salary: each is counted as shown in the
// log of who was shown a salary, so a query with one is read again when it
// is asked for or after a change, never just because the window got focus
// (refetchOnWindowFocus: (q) => !candidateShowsSalary(q.state.data)).
import type { Schemas } from "./client";

/** The candidate came back with their current or expected salary. */
export function candidateShowsSalary(c: Schemas["Candidate"] | undefined): boolean {
  return !!(c?.compensation?.currentSalary ?? c?.compensation?.expectedSalary);
}

/** A screening call came back with the salary it took. */
export function callsShowSalary(calls: Schemas["ScreeningCallSummary"][] | undefined): boolean {
  return !!calls?.some((c) => c.salarySnapshot);
}
