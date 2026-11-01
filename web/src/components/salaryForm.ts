// The salary form's state, and its conversion to the API request.
import type { Compensation, Schemas } from "../api/client";

export interface SalaryFormState {
  currentAmount: string;
  currentCurrency: string;
  currentPeriod: Schemas["PayPeriod"];
  expectedMin: string;
  expectedMax: string;
  expectedCurrency: string;
  expectedPeriod: Schemas["PayPeriod"];
}

export function formFromCompensation(c: Compensation | null | undefined, defaultCurrency = "MYR"): SalaryFormState {
  return {
    currentAmount: c?.currentSalary?.amount ?? "",
    currentCurrency: c?.currentSalary?.currency ?? defaultCurrency,
    currentPeriod: c?.currentSalary?.period ?? "month",
    expectedMin: c?.expectedSalary?.min ?? "",
    expectedMax: c?.expectedSalary?.max ?? "",
    expectedCurrency: c?.expectedSalary?.currency ?? c?.currentSalary?.currency ?? defaultCurrency,
    expectedPeriod: c?.expectedSalary?.period ?? "month",
  };
}

/** "8,500" -> "8500": people type thousands separators; the API wants plain numbers. */
export function plainAmount(value: string): string {
  return value.replace(/[\s,]/g, "");
}

/** The PUT body. An empty amount means "not recorded". */
export function toCompensationBody(f: SalaryFormState, expectedVersion: number): Schemas["CompensationUpdate"] {
  const current = plainAmount(f.currentAmount);
  const min = plainAmount(f.expectedMin);
  const max = plainAmount(f.expectedMax);
  return {
    expectedVersion,
    currentSalary: current
      ? { amount: current, currency: f.currentCurrency.trim().toUpperCase(), period: f.currentPeriod }
      : null,
    expectedSalary: min
      ? { min, max: max || null, currency: f.expectedCurrency.trim().toUpperCase(), period: f.expectedPeriod }
      : null,
  };
}
