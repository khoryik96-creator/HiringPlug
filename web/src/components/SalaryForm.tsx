import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Candidate, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { CURRENCIES, fieldErrors, PERIODS } from "./forms";
import { formFromCompensation, type SalaryFormState, toCompensationBody } from "./salaryForm";

/** Edit current and expected salary (needs candidates.compensation:update). */
export function SalaryForm({ candidate, onDone }: { candidate: Candidate; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SalaryFormState>(() => formFromCompensation(candidate.compensation));
  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PUT("/v1/candidates/{candidateId}/compensation", {
          params: { path: { candidateId: candidate.id } },
          body: toCompensationBody(form, candidate.compensation?.version ?? 0),
        }),
      ),
    onSuccess: (compensation) => {
      queryClient.setQueryData<Candidate>(["candidate", candidate.id], (c) => (c ? { ...c, compensation } : c));
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };
  const amount = (key: "currentAmount" | "expectedMin" | "expectedMax", label: string, error?: string) => (
    <Field label={label} error={error}>
      <input inputMode="decimal" placeholder="e.g. 8500" value={form[key]} onChange={(e) => { set(key, e.target.value); }} />
    </Field>
  );
  const currency = (key: "currentCurrency" | "expectedCurrency") => (
    <Field label="Currency">
      <input list="currencies" maxLength={3} value={form[key]} onChange={(e) => { set(key, e.target.value.toUpperCase()); }} />
    </Field>
  );
  const period = (key: "currentPeriod" | "expectedPeriod") => (
    <Field label="Per">
      <select value={form[key]} onChange={(e) => { set(key, e.target.value as SalaryFormState[typeof key]); }}>
        {PERIODS.map((p) => <option key={p} value={p}>{p}</option>)}
      </select>
    </Field>
  );
  const stale = save.error instanceof ApiError && save.error.code === "CONFLICT_STALE_VERSION";

  return (
    <form
      className="form-grid"
      aria-label="Edit salary"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <datalist id="currencies">
        {CURRENCIES.map((c) => <option key={c} value={c} />)}
      </datalist>
      <strong className="full">Current salary</strong>
      {amount("currentAmount", "Amount", errors["currentSalary.amount"])}
      {currency("currentCurrency")}
      {period("currentPeriod")}
      <strong className="full">Expected salary (leave "to" empty for one figure)</strong>
      {amount("expectedMin", "From", errors["expectedSalary.min"])}
      {amount("expectedMax", "To", errors["expectedSalary.max"])}
      {currency("expectedCurrency")}
      {period("expectedPeriod")}
      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save salary</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      {stale ? (
        <p className="error full">
          Someone else changed the salary while you were editing.{" "}
          <button type="button" className="link" onClick={() => { void queryClient.invalidateQueries({ queryKey: ["candidate", candidate.id] }); onDone(); }}>
            Reload their version
          </button>
        </p>
      ) : (
        Object.keys(errors).length === 0 && <div className="full"><ErrorMessage error={save.error} /></div>
      )}
    </form>
  );
}
