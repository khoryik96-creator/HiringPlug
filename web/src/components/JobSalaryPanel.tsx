import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type JobOrder, type Schemas, unwrap } from "../api/client";
import { jobKey } from "../api/keys";
import { keysOf, PAY_PERIOD } from "../labels";
import { Field } from "./Field";
import { CURRENCIES, fieldErrors, FormInvalid, sameBody } from "./forms";
import { plainAmount } from "./salaryFormState";
import { SaveError } from "./SaveError";

type Range = Schemas["JobSalaryRange"];

export function formatBudget(r: Range): string {
  return `${r.currency} ${r.max ? `${r.min} – ${r.max}` : r.min} / ${PAY_PERIOD[r.period]}`;
}

interface BudgetForm { min: string; max: string; currency: string; period: Schemas["PayPeriod"] }

const PATHS = { min: "range.min", max: "range.max", currency: "range.currency", period: "range.period" } as const;

function budgetBody(f: BudgetForm): Range | null {
  const min = plainAmount(f.min);
  if (min === "" && plainAmount(f.max) === "") return null;
  return { min, max: plainAmount(f.max) || null, currency: f.currency.trim().toUpperCase(), period: f.period };
}

function budgetProblems(f: BudgetForm): Record<string, string> {
  const problems: Record<string, string> = {};
  for (const key of ["min", "max"] as const) {
    const v = plainAmount(f[key]);
    if (v !== "" && !/^(0|[1-9]\d*)(\.\d{1,2})?$/.test(v)) problems[PATHS[key]] = "Enter an amount like 8500 or 8,500.50 (a point before cents).";
  }
  if (plainAmount(f.min) === "" && plainAmount(f.max) !== "") problems[PATHS.min] = 'Enter the lower figure here. For a single figure, fill in "From" only.';
  if (plainAmount(f.min) !== "" && !/^[A-Z]{3}$/.test(f.currency.trim().toUpperCase())) problems[PATHS.currency] = "Use the 3-letter currency code, e.g. MYR (not RM).";
  return problems;
}

/** A job's salary budget, or a "restricted" notice for roles that may not see it. */
export function JobSalaryPanel({ job, canEdit }: { job: JobOrder; canEdit: boolean }) {
  const [editing, setEditing] = useState(false);
  if (job.restrictedFields.includes("salary")) {
    return (
      <section className="panel" aria-label="Salary budget">
        <h3>Salary budget</h3>
        <p className="muted">🔒 The salary budget is restricted for your role.</p>
      </section>
    );
  }
  const range = job.salary?.range;
  return (
    <section className="panel" aria-label="Salary budget">
      <div className="row spread">
        <h3>Salary budget</h3>
        {canEdit && !editing && <button type="button" onClick={() => { setEditing(true); }}>{range ? "Edit budget" : "Add budget"}</button>}
      </div>
      {editing ? (
        <BudgetForm job={job} onDone={() => { setEditing(false); }} />
      ) : (
        <p className={range ? "" : "muted"}>{range ? formatBudget(range) : "None recorded."}</p>
      )}
    </section>
  );
}

function BudgetForm({ job, onDone }: { job: JobOrder; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [opened] = useState(() => {
    const r = job.salary?.range;
    const form: BudgetForm = { min: r?.min ?? "", max: r?.max ?? "", currency: r?.currency ?? "MYR", period: r?.period ?? "month" };
    return { id: job.id, version: job.salary?.version ?? 0, form };
  });
  const [form, setForm] = useState(opened.form);
  const save = useMutation({
    mutationFn: async () => {
      const problems = budgetProblems(form);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const range = budgetBody(form);
      if (sameBody(range, budgetBody(opened.form))) return null;
      return unwrap(await api.PUT("/v1/jobs/{jobId}/salary", {
        params: { path: { jobId: opened.id } }, body: { expectedVersion: opened.version, range },
      }));
    },
    onSuccess: (salary) => {
      if (salary) {
        queryClient.setQueryData<JobOrder>(jobKey(opened.id), (j) => (j ? { ...j, salary: salary.range ? salary : null } : j));
      }
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const input = (key: "min" | "max" | "currency", label: string) => (
    <Field label={label} error={errors[PATHS[key]]}>
      <input
        value={form[key]}
        className={key === "currency" ? "code" : undefined}
        list={key === "currency" ? "budget-currencies" : undefined}
        inputMode={key === "currency" ? undefined : "decimal"}
        onChange={(e) => { setForm((f) => ({ ...f, [key]: e.target.value })); }}
      />
    </Field>
  );
  return (
    <form className="form-grid" aria-label="Edit salary budget" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <datalist id="budget-currencies">{CURRENCIES.map((c) => <option key={c} value={c} />)}</datalist>
      {input("min", "From (or the single figure)")}
      {input("max", "To (optional)")}
      {input("currency", "Currency")}
      <Field label="Per" error={errors[PATHS.period]}>
        <select value={form.period} onChange={(e) => { setForm((f) => ({ ...f, period: e.target.value as BudgetForm["period"] })); }}>
          {keysOf(PAY_PERIOD).map((p) => <option key={p} value={p}>{PAY_PERIOD[p]}</option>)}
        </select>
      </Field>
      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save budget</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError
        error={save.error}
        shown={Object.values(PATHS)}
        onReload={() => { void queryClient.invalidateQueries({ queryKey: jobKey(opened.id) }); onDone(); }}
      />
    </form>
  );
}
