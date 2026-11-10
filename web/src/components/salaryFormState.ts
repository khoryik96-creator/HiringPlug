// The salary form's state, its checks, and its conversion to the API request.
import type { Compensation, Schemas } from "../api/client";

export interface SalaryFormState {
  currentDescription?: string;
  expectedDescription?: string;
  calculationPending?: boolean;
  currentNeedsReview?: boolean;
  expectedNeedsReview?: boolean;
  currentAmount: string;
  currentCurrency: string;
  currentPeriod: Schemas["PayPeriod"];
  expectedMin: string;
  expectedMax: string;
  expectedCurrency: string;
  expectedPeriod: Schemas["PayPeriod"];
}

/** Where the API reports a problem with each input (the VALIDATION_FAILED field path). */
export const SALARY_PATHS: Record<keyof SalaryFormState, string> = {
  currentDescription: "currentSalary.description",
  expectedDescription: "expectedSalary.description",
  calculationPending: "currentSalary.description",
  currentNeedsReview: "currentSalary.description",
  expectedNeedsReview: "expectedSalary.description",
  currentAmount: "currentSalary.amount",
  currentCurrency: "currentSalary.currency",
  currentPeriod: "currentSalary.period",
  expectedMin: "expectedSalary.min",
  expectedMax: "expectedSalary.max",
  expectedCurrency: "expectedSalary.currency",
  expectedPeriod: "expectedSalary.period",
};

const DEFAULT_CURRENCY = "MYR";

export function formFromCompensation(c: Compensation | null | undefined): SalaryFormState {
  // A figure not recorded yet starts with the other figure's currency and
  // period: a yearly SGD salary is most likely expected yearly in SGD too.
  const current = c?.currentSalary;
  const expected = c?.expectedSalary;
  return {
    ...(current?.description !== undefined ? { currentDescription: current.description } : {}),
    ...(expected?.description !== undefined ? { expectedDescription: expected.description } : {}),
    currentAmount: current?.amount ?? "",
    currentCurrency: current?.currency ?? expected?.currency ?? DEFAULT_CURRENCY,
    currentPeriod: current?.period ?? expected?.period ?? "month",
    expectedMin: expected?.min ?? "",
    expectedMax: expected?.max ?? "",
    expectedCurrency: expected?.currency ?? current?.currency ?? DEFAULT_CURRENCY,
    expectedPeriod: expected?.period ?? current?.period ?? "month",
  };
}

/**
 * "8,500" -> "8500", "1,00,000" -> "100000", "10 000" -> "10000". A comma
 * counts as a thousands separator only when 3 digits follow the last one, so
 * "8500,50" (a decimal comma) is left as typed and refused, not read as 850050.
 */
export function plainAmount(value: string): string {
  const v = value.replace(/\s/g, "");
  return /^\d{1,3}(,\d{2,3})*,\d{3}(\.\d+)?$/.test(v) ? v.replace(/,/g, "") : v;
}

const AMOUNT = /^(0|[1-9]\d*)(\.\d{1,2})?$/;
const CURRENCY = /^[A-Z]{3}$/;

/**
 * Mistakes worth catching before sending, keyed like the API's field errors.
 * The API checks everything again; this only gives clearer messages.
 */
export function salaryProblems(f: SalaryFormState): Record<string, string> {
  const problems: Record<string, string> = {};
  if (f.calculationPending) problems[f.expectedNeedsReview && !f.currentNeedsReview ? "expectedSalary.description" : "currentSalary.description"] = "Calculate and review the salary before saving.";
  for (const key of ["currentAmount", "expectedMin", "expectedMax"] as const) {
    const amount = plainAmount(f[key]);
    if (amount !== "" && !AMOUNT.test(amount)) {
      problems[SALARY_PATHS[key]] = "Enter an amount like 8500 or 8,500.50 (a point before cents).";
    }
  }
  if (plainAmount(f.expectedMin) === "" && plainAmount(f.expectedMax) !== "") {
    problems[SALARY_PATHS.expectedMin] = 'Enter the lower figure here. For a single figure, fill in "From" only.';
  }
  const currencies = [
    ["currentCurrency", f.currentAmount],
    ["expectedCurrency", f.expectedMin],
  ] as const;
  for (const [key, amount] of currencies) {
    if (plainAmount(amount) !== "" && !CURRENCY.test(f[key].trim().toUpperCase())) {
      problems[SALARY_PATHS[key]] = "Use the 3-letter currency code, e.g. MYR (not RM).";
    }
  }
  return problems;
}

/** The PUT body. An empty amount means "not recorded". */
export function toCompensationBody(f: SalaryFormState, expectedVersion: number): Schemas["CompensationUpdate"] {
  const current = plainAmount(f.currentAmount);
  const min = plainAmount(f.expectedMin);
  const max = plainAmount(f.expectedMax);
  return {
    expectedVersion,
    currentSalary: current
      ? { ...(f.currentDescription !== undefined ? { description: f.currentDescription } : {}), amount: current, currency: f.currentCurrency.trim().toUpperCase(), period: f.currentPeriod }
      : null,
    expectedSalary: min
      ? { ...(f.expectedDescription !== undefined ? { description: f.expectedDescription } : {}), min, max: max || null, currency: f.expectedCurrency.trim().toUpperCase(), period: f.expectedPeriod }
      : null,
  };
}
