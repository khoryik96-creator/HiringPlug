import type { Candidate, Compensation } from "../api/client";

type Salary = NonNullable<Compensation["currentSalary"]>;
type Range = NonNullable<Compensation["expectedSalary"]>;

const PERIOD: Record<Salary["period"], string> = { hour: "hour", day: "day", month: "month", year: "year" };

function formatSalary(s: Salary): string {
  return `${s.currency} ${s.amount} / ${PERIOD[s.period]}`;
}

function formatRange(r: Range): string {
  const amount = r.max ? `${r.min} – ${r.max}` : r.min;
  return `${r.currency} ${amount} / ${PERIOD[r.period]}`;
}

/** Salary details, or a clear "restricted" notice for roles that may not see them. */
export function SalaryPanel({ candidate }: { candidate: Candidate }) {
  if (candidate.restrictedFields.includes("compensation")) {
    return (
      <section className="panel" aria-label="Salary">
        <h3>Salary</h3>
        <p className="muted">🔒 Salary details are restricted for your role.</p>
      </section>
    );
  }
  const comp = candidate.compensation;
  return (
    <section className="panel" aria-label="Salary">
      <h3>Salary</h3>
      {!comp ? (
        <p className="muted">None recorded.</p>
      ) : (
        <dl>
          <dt>Current</dt>
          <dd>{comp.currentSalary ? formatSalary(comp.currentSalary) : "—"}</dd>
          <dt>Expected</dt>
          <dd>{comp.expectedSalary ? formatRange(comp.expectedSalary) : "—"}</dd>
        </dl>
      )}
    </section>
  );
}
