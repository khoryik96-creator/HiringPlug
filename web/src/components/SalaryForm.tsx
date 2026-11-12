import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { keysOf, PAY_PERIOD } from "../labels";
import { Field } from "./Field";
import { CURRENCIES, fieldErrors, FormInvalid, sameBody } from "./forms";
import {
  figuresOnly,
  formFromCompensation,
  SALARY_PATHS,
  salaryProblems,
  type SalaryFormState,
  toCompensationBody,
} from "./salaryFormState";
import { SaveError } from "./SaveError";

/** Edit current and expected salary (needs candidates.compensation:update). */
export function SalaryForm({ candidate, onDone }: { candidate: Candidate; onDone: () => void }) {
  const queryClient = useQueryClient();
  // What the user started from; the version is kept for the same reason as in CandidateForm.
  const [opened] = useState(() => ({
    id: candidate.id,
    version: candidate.compensation?.version ?? 0,
    form: figuresOnly(formFromCompensation(candidate.compensation)),
  }));
  const [form, setForm] = useState(opened.form);
  const save = useMutation({
    mutationFn: async () => {
      // Checked first: the body leaves out what it cannot express (a "To" without "From").
      const problems = salaryProblems(form);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const body = toCompensationBody(form, opened.version);
      // Nothing changed (including "Add salary" saved empty): don't write anything.
      if (sameBody(body, toCompensationBody(opened.form, opened.version))) return null;
      return unwrap(
        await api.PUT("/v1/candidates/{candidateId}/compensation", { params: { path: { candidateId: opened.id } }, body }),
      );
    },
    onSuccess: (compensation) => {
      if (compensation) {
        queryClient.setQueryData<Candidate>(candidateKey(opened.id), (c) => (c ? { ...c, compensation } : c));
      }
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };

  return (
    <form
      className="form-grid"
      aria-label="Edit salary"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <SalaryFields form={form} set={set} errors={errors} />
      <div className="full"><Field label="Salary notes" error={errors.salaryNotes}><textarea rows={3} maxLength={10000} value={form.salaryNotes ?? ""} onChange={(e) => { set("salaryNotes",e.target.value); }} /></Field></div>
      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save salary</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError
        error={save.error}
        shown={Object.values(SALARY_PATHS)}
        onReload={() => { void queryClient.invalidateQueries({ queryKey: candidateKey(opened.id) }); onDone(); }}
      />
    </form>
  );
}

/** The current and expected salary inputs (also on the screening call). */
export function SalaryFields({ form, set, errors }: {
  form: SalaryFormState;
  set: <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => void;
  errors: Record<string, string>;
}) {
  const amount = (key: "currentAmount" | "expectedMin" | "expectedMax", label: string) => (
    <Field label={label} error={errors[SALARY_PATHS[key]]}>
      <input inputMode="decimal" value={form[key]} onChange={(e) => { set(key, e.target.value); }} />
    </Field>
  );
  const currency = (key: "currentCurrency" | "expectedCurrency") => (
    <Field label="Currency" error={errors[SALARY_PATHS[key]]}>
      {/* Shown in capitals by CSS and upper-cased when sent: rewriting the value
          on each key press garbles input on some phone keyboards. */}
      <input list="currencies" className="code" value={form[key]} onChange={(e) => { set(key, e.target.value); }} />
    </Field>
  );
  const period = (key: "currentPeriod" | "expectedPeriod") => (
    <Field label="Per" error={errors[SALARY_PATHS[key]]}>
      <select value={form[key]} onChange={(e) => { set(key, e.target.value as SalaryFormState[typeof key]); }}>
        {keysOf(PAY_PERIOD).map((p) => <option key={p} value={p}>{PAY_PERIOD[p]}</option>)}
      </select>
    </Field>
  );
  return (
    <>
      <datalist id="currencies">
        {CURRENCIES.map((c) => <option key={c} value={c} />)}
      </datalist>
      <fieldset className="form-grid full">
        <legend>Current salary</legend>
        {amount("currentAmount", "Amount")}
        {currency("currentCurrency")}
        {period("currentPeriod")}
      </fieldset>
      <fieldset className="form-grid full">
        <legend>Expected salary</legend>
        {amount("expectedMin", "From (or the single figure)")}
        {amount("expectedMax", "To (optional)")}
        {currency("expectedCurrency")}
        {period("expectedPeriod")}
      </fieldset>
    </>
  );
}
