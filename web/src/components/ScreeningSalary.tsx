import { useMutation } from "@tanstack/react-query";
import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { SalaryFields } from "./SalaryForm";
import { useSalaryRules } from "./SalaryCalculationSettings";
import type { SalaryFormState } from "./salaryFormState";

export function ScreeningSalary({ form, patch, set, errors }: {
  form: SalaryFormState; patch: (value: Partial<SalaryFormState>) => void;
  set: <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => void;
  errors: Record<string, string>;
}) {
  const rules = useSalaryRules();
  const current = form.currentDescription ?? (form.currentAmount ? `${form.currentCurrency} ${form.currentAmount} per ${form.currentPeriod}` : "");
  const expected = form.expectedDescription ?? (form.expectedMin ? `${form.expectedCurrency} ${form.expectedMin}${form.expectedMax ? ` - ${form.expectedMax}` : ""} per ${form.expectedPeriod}` : "");
  const calculate = useMutation({
    mutationFn: async (body: Schemas["SalaryCalculationInput"]) => unwrap(await api.POST("/v1/salary-calculations", { body })),
  });
  const result = calculate.data;
  const amount = (v: string) => Number(v).toLocaleString("en-MY", { maximumFractionDigits: 2 });
  const currency = (code: string) => code.toUpperCase() === "MYR" ? "RM" : code.toUpperCase();
  const fresh = result && calculate.variables.current === current && calculate.variables.expected === expected;
  const edit = (value: Partial<SalaryFormState>) => { patch({ ...value, calculationPending: true }); calculate.reset(); };
  const useFigures = () => {
    if (!fresh) return;
    patch({ currentAmount: result.monthly, currentPeriod: "month", currentCurrency: result.currentCurrency ?? form.currentCurrency, currentDescription: current,
      expectedMin: result.expectedMin ?? "", expectedMax: result.expectedMax ?? "", expectedPeriod: "month", expectedCurrency: result.expectedCurrency ?? form.expectedCurrency, expectedDescription: expected, calculationPending: false });
  };
  return <section className="full screening-salary" aria-label="Salary calculation">
    <div className="row"><h3>Salary</h3></div>
    <Field label="Current salary breakdown" error={errors["currentSalary.description"]}>
      <textarea rows={3} maxLength={2000} value={current} onChange={(e) => { edit({ currentDescription: e.target.value }); }} />
    </Field>
    <Field label="Expected salary" error={errors["expectedSalary.description"]}>
      <input maxLength={2000} value={expected} onChange={(e) => { edit({ expectedDescription: e.target.value }); }} />
    </Field>
    <div className="row"><button type="button" disabled={!rules.data || calculate.isPending || !current.trim()} onClick={() => {
      if (rules.data) calculate.mutate({ current, expected, rules: rules.data });
    }}>Calculate</button></div>
    <ErrorMessage error={rules.error ?? calculate.error} />
    {fresh && <div className="salary-breakdown">
      <h4>Fixed monthly calculation</h4>
      <dl><dt>Basic salary</dt><dd>{amount(result.base)}</dd><dt>Fixed allowances</dt><dd>{amount(result.allowances)}</dd>
        <dt>Guaranteed bonus / month <small>({result.annualMonths} months total)</small></dt><dd>{amount(result.bonus)}</dd></dl>
      <div className="salary-total"><strong>Fixed monthly equivalent</strong><span>{currency(result.currentCurrency ?? form.currentCurrency)} {amount(result.monthly)}</span></div>
      {result.expectedMin && <p>Expected monthly: {currency(result.expectedCurrency ?? form.expectedCurrency)} {amount(result.expectedMin)}{result.expectedMax ? ` – ${amount(result.expectedMax)}` : ""}</p>}
      {result.excluded.length > 0 && <p className="muted small">Excluded: {result.excluded.join(" · ")}</p>}
      {result.warnings.map((w) => <p className="small" key={w}>{w}</p>)}
      <button type="button" disabled={Number(result.base) <= 0 || result.warnings.some((w) => /Working hours|Conflicting|Enter the guaranteed|could not be read/.test(w)) || (!!expected.trim() && !result.expectedMin)} onClick={useFigures}>Use these figures</button>
    </div>}
    <details><summary>Salary figures and currencies</summary><div className="form-grid"><SalaryFields form={form} set={(key, value) => { set(key, value); if (key === "currentAmount" || key === "expectedMin" || key === "expectedMax") patch({ calculationPending: false }); }} errors={errors} /></div></details>
    {form.calculationPending && !fresh && <p className="small">Calculate and review the salary before saving.</p>}
  </section>;
}
