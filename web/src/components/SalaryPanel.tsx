import { useState } from "react";

import type { Candidate, Compensation } from "../api/client";
import { PAY_PERIOD } from "../labels";
import { SalaryForm } from "./SalaryForm";

type Salary = NonNullable<Compensation["currentSalary"]>;
type Range = NonNullable<Compensation["expectedSalary"]>;

function formatSalary(s: Salary): string {
  return `${s.currency} ${s.amount} / ${PAY_PERIOD[s.period]}`;
}

function formatRange(r: Range): string {
  const amount = r.max ? `${r.min} – ${r.max}` : r.min;
  return `${r.currency} ${amount} / ${PAY_PERIOD[r.period]}`;
}

/** Salary details, or a clear "restricted" notice for roles that may not see them. */
export function SalaryPanel({ candidate, canEdit = false }: { candidate: Candidate; canEdit?: boolean }) {
  const [editing, setEditing] = useState(false);
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
      <div className="row spread">
        <h3>Salary</h3>
        {canEdit && !editing && (
          <button type="button" onClick={() => { setEditing(true); }}>
            {comp ? "Edit salary" : "Add salary"}
          </button>
        )}
      </div>
      {editing ? (
        <SalaryForm candidate={candidate} onDone={() => { setEditing(false); }} />
      ) : !comp ? (
        <p className="muted">None recorded.</p>
      ) : (
        <dl>
          <dt>Current</dt>
          <dd>{comp.currentSalary ? formatSalary(comp.currentSalary) : "—"}</dd>
          {comp.currentSalary?.description && <><dt>Current breakdown</dt><dd className="pre">{comp.currentSalary.description}</dd></>}
          <dt>Expected</dt>
          <dd>{comp.expectedSalary ? formatRange(comp.expectedSalary) : "—"}</dd>
          {comp.expectedSalary?.description && <><dt>Expected salary notes</dt><dd className="pre">{comp.expectedSalary.description}</dd></>}
          {comp.salaryNotes && <><dt>Salary notes</dt><dd className="pre">{comp.salaryNotes}</dd></>}
        </dl>
      )}
    </section>
  );
}
