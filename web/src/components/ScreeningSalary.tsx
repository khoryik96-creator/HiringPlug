// The screening call's salary: the recruiter types the breakdown as said on
// the call, the server calculates a fixed monthly figure with the company's
// saved rules, and the recruiter applies it (or enters reviewed figures).
import { useMutation } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { SalaryFields } from "./SalaryForm";
import { salaryProblems, type SalaryFormState } from "./salaryFormState";

type Calculation = Schemas["SalaryCalculation"];

const amount = (v: string) => Number(v).toLocaleString("en-MY", { maximumFractionDigits: 2 });
const currency = (code: string) => (code.toUpperCase() === "MYR" ? "RM" : code.toUpperCase());

export function ScreeningSalary({ form, patch, set, errors }: {
  form: SalaryFormState;
  patch: (value: Partial<SalaryFormState>) => void;
  set: <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => void;
  errors: Record<string, string>;
}) {
  const manualFields = useRef<HTMLDetailsElement>(null);
  const blockedId = useId();
  const [manualErrors, setManualErrors] = useState<Record<string, string>>({});
  const [applied, setApplied] = useState(false);
  const current = form.currentDescription ?? describeCurrent(form);
  const expected = form.expectedDescription ?? describeExpected(form);
  const selectedCurrency = form.currentCurrency.trim().toUpperCase();

  const calculate = useMutation({
    mutationFn: async (body: Schemas["SalaryCalculationInput"]) => unwrap(await api.POST("/v1/salary-calculations", { body })),
  });
  const result = calculate.data;
  const asked = calculate.variables;
  // A result counts only for the wording and currency it was calculated from.
  const fresh = result && asked && asked.current === current && asked.expected === expected
    && ((asked.currentCurrency ?? "") === selectedCurrency || result.currentCurrency === selectedCurrency);
  // Points to check: shown highlighted, but the recruiter decides and may
  // still use the figures. Only a result with no figure at all has nothing to use.
  const needsReview = result && (result.reviewRequired || (!!expected.trim() && !result.expectedMin));
  const nothingToUse = result && !result.hasCurrent && !result.expectedMin;

  const edit = (value: Partial<SalaryFormState>) => {
    setApplied(false);
    setManualErrors({});
    patch({
      ...value,
      ...(value.currentDescription !== undefined ? { currentNeedsReview: true } : {}),
      ...(value.expectedDescription !== undefined ? { expectedNeedsReview: true } : {}),
      calculationPending: true,
    });
    calculate.reset();
  };

  const useFigures = () => {
    if (!fresh || nothingToUse) return;
    // A figure the calculation could not read keeps what the form had.
    const expectedRead = !!result.expectedMin;
    patch({
      currentAmount: result.hasCurrent ? result.monthly : form.currentAmount,
      currentPeriod: result.hasCurrent ? "month" : form.currentPeriod,
      currentCurrency: result.currentCurrency ?? form.currentCurrency,
      currentDescription: current,
      expectedMin: expectedRead ? result.expectedMin ?? "" : form.expectedMin,
      expectedMax: expectedRead ? result.expectedMax ?? "" : form.expectedMax,
      expectedPeriod: expectedRead ? "month" : form.expectedPeriod,
      expectedCurrency: result.expectedCurrency ?? form.expectedCurrency,
      expectedDescription: expected,
      calculationPending: false,
      currentNeedsReview: false,
      expectedNeedsReview: false,
    });
    setApplied(true);
  };

  const useManualFigures = () => {
    const problems = salaryProblems({ ...form, calculationPending: false });
    if (current.trim() && !form.currentAmount.trim()) {
      problems["currentSalary.amount"] = "Enter a reviewed current amount, or clear its description.";
    }
    if (expected.trim() && !form.expectedMin.trim()) {
      problems["expectedSalary.min"] = "Enter a reviewed expected amount, or clear its description.";
    }
    setManualErrors(problems);
    if (Object.keys(problems).length) return;
    patch({ calculationPending: false, currentNeedsReview: false, expectedNeedsReview: false });
    calculate.reset();
    setApplied(true);
  };

  const openManual = () => {
    if (!manualFields.current) return;
    manualFields.current.open = true;
    manualFields.current.querySelector<HTMLInputElement>("input")?.focus();
  };

  // A figure typed by hand: an expectation that depends on the current salary
  // ("10% increment") has to be checked again.
  const manual = <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => {
    setApplied(false);
    setManualErrors({});
    set(key, value);
    calculate.reset();
    const relative = dependsOnCurrent(expected);
    if (key === "currentAmount") {
      const recheck = !!form.expectedNeedsReview || relative;
      patch({ currentNeedsReview: false, expectedNeedsReview: recheck, calculationPending: recheck });
    }
    if ((key === "currentCurrency" || key === "currentPeriod") && relative) {
      patch({ expectedNeedsReview: true, calculationPending: true });
    }
    if (key === "expectedMin" || key === "expectedMax") {
      patch({ expectedNeedsReview: false, calculationPending: !!form.currentNeedsReview });
    }
  };

  return (
    <section className="full screening-salary" aria-label="Salary calculation">
      <div className="row"><h3>Salary</h3></div>
      <Field label="Current salary breakdown" error={errors["currentSalary.description"]}>
        <textarea rows={3} maxLength={2000} value={current} onChange={(e) => { edit({ currentDescription: e.target.value }); }} />
      </Field>
      <Field label="Expected salary" error={errors["expectedSalary.description"]}>
        <input maxLength={2000} value={expected} onChange={(e) => { edit({ expectedDescription: e.target.value }); }} />
      </Field>
      <Field label="Salary notes" error={errors.salaryNotes}>
        <textarea rows={3} maxLength={10000} value={form.salaryNotes ?? ""} onChange={(e) => { set("salaryNotes", e.target.value); }} />
      </Field>
      <div className="row">
        <button
          type="button"
          disabled={calculate.isPending || (!current.trim() && !expected.trim())}
          onClick={() => {
            setApplied(false);
            calculate.mutate({ current, expected, ...(selectedCurrency ? { currentCurrency: selectedCurrency } : {}) });
          }}
        >
          Calculate
        </button>
      </div>
      <ErrorMessage error={calculate.error} />
      {!fresh && !!calculate.error && <button type="button" onClick={openManual}>Enter figures manually</button>}

      {fresh && (
        <div className="salary-breakdown">
          <h4>Fixed monthly calculation</h4>
          {result.hasCurrent && <Breakdown result={result} />}
          <div className="salary-total">
            <strong>Fixed monthly equivalent</strong>
            <span>{result.hasCurrent ? `${currency(result.currentCurrency ?? form.currentCurrency)} ${amount(result.monthly)}` : "—"}</span>
          </div>
          {result.expectedMin && (
            <p>
              Expected monthly: {currency(result.expectedCurrency ?? form.expectedCurrency)} {amount(result.expectedMin)}
              {result.expectedMax ? ` – ${amount(result.expectedMax)}` : ""}
            </p>
          )}
          {result.excluded.length > 0 && <p className="muted small">Excluded: {result.excluded.join(" · ")}</p>}
          {result.warnings.map((w) => <p className={needsReview ? "warning small" : "small"} key={w}>{w}</p>)}
          {needsReview && (
            <p className="warning small" id={blockedId} role="alert">
              Check the points above. You can still use these figures, or enter reviewed figures manually.
            </p>
          )}
          <div className="row">
            <button type="button" disabled={!!nothingToUse} aria-describedby={needsReview ? blockedId : undefined} onClick={useFigures}>
              Use these figures
            </button>
            {(needsReview || nothingToUse) && <button type="button" onClick={openManual}>Enter figures manually</button>}
          </div>
        </div>
      )}

      <details ref={manualFields}>
        <summary>Salary figures and currencies</summary>
        <div className="form-grid">
          <SalaryFields form={form} set={manual} errors={{ ...errors, ...manualErrors }} />
        </div>
        <button type="button" onClick={useManualFigures}>Use entered figures</button>
      </details>
      {applied && <p className="small" role="status">Figures applied. Save the call.</p>}
      {form.calculationPending && !fresh && <p className="small">Calculate and review the salary before saving.</p>}
    </section>
  );
}

function Breakdown({ result }: { result: Calculation }) {
  return (
    <dl>
      <dt>Basic salary</dt>
      <dd>{amount(result.base)}</dd>
      <dt>Fixed allowances</dt>
      <dd>{amount(result.allowances)}</dd>
      <dt>Guaranteed bonus / month <small>({result.annualMonths} months total)</small></dt>
      <dd>{amount(result.bonus)}</dd>
    </dl>
  );
}

/** Saved figures shown as wording when no breakdown was recorded: "MYR 8500 per month". */
function describeCurrent(f: SalaryFormState): string {
  return f.currentAmount ? `${f.currentCurrency} ${f.currentAmount} per ${f.currentPeriod}` : "";
}

function describeExpected(f: SalaryFormState): string {
  if (!f.expectedMin) return "";
  const range = f.expectedMax ? `${f.expectedMin} - ${f.expectedMax}` : f.expectedMin;
  return `${f.expectedCurrency} ${range} per ${f.expectedPeriod}`;
}

// Dependency validation only; all monetary calculations stay in Go.
function dependsOnCurrent(wording: string): boolean {
  const normalized = wording
    .replace(/(\d)\s*(?:pct|per\s+cent|percen|percentage|percent)\b/gi, "$1%")
    .replace(/—/g, "–")
    .replace(/\bplus\b/gi, "+");
  const primary = normalized.split(/[+;,]/).find((part) => part.trim() && !/bonus|commission|performance/i.test(part)) ?? "";
  const percentage = /[-+]?\d+(?:\.\d+)?\s*(?:%|percent)?\s*(?:-|–|~|to)\s*[-+]?\d+(?:\.\d+)?\s*(?:%|percent)|[-+]?\d+(?:\.\d+)?\s*(?:%|percent)/gi;
  const withoutPercent = primary.replace(percentage, "");
  const relative = /%|percent|\b(?:same|similar|lateral|horizontal|maintain|match|follow)\b|no\s+(?:increment|increase|change)|as\s+current|current\s+(?:salary|package)/i;
  return !/\d/.test(withoutPercent) && relative.test(primary);
}
