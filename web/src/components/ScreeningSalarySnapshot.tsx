import type { Schemas } from "../api/client";
import { PAY_PERIOD } from "../labels";

/** Recorded values stay with their call even after the profile salary is edited. */
export function ScreeningSalarySnapshot({ value }: { value: Schemas["ScreeningSalarySnapshot"] }) {
 const current=value.currentSalary,expected=value.expectedSalary;
 return <div className="call-salary"><dl>
  {current && <><dt>Current salary</dt><dd>{current.currency} {current.amount} / {PAY_PERIOD[current.period]}</dd>{current.description && <><dt>Current breakdown</dt><dd className="pre">{current.description}</dd></>}</>}
  {expected && <><dt>Expected salary</dt><dd>{expected.currency} {expected.min}{expected.max && expected.max!==expected.min ? ` – ${expected.max}` : ""} / {PAY_PERIOD[expected.period]}</dd>{expected.description && <><dt>Expected salary notes</dt><dd className="pre">{expected.description}</dd></>}</>}
  {value.salaryNotes && <><dt>Salary notes</dt><dd className="pre">{value.salaryNotes}</dd></>}
 </dl></div>;
}
