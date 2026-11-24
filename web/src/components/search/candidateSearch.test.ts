import { describe, expect, it } from "vitest";

import { EMPTY, wantsSalary } from "./candidateSearch";

describe("wantsSalary", () => {
  const plain = ["currentRole", "status", "email"];

  it("asks for salaries only when the list shows or uses them", () => {
    expect(wantsSalary(true, [...plain, "expectedSalary"], "newest", EMPTY)).toBe(true);
    expect(wantsSalary(true, [...plain, "currentSalary"], null, EMPTY)).toBe(true);
    expect(wantsSalary(true, plain, "newest", EMPTY)).toBe(false);
    expect(wantsSalary(true, plain, null, EMPTY)).toBe(false);
    expect(wantsSalary(true, plain, "current_salary_desc", EMPTY)).toBe(true);
    expect(wantsSalary(true, plain, "name", EMPTY)).toBe(false);
    expect(wantsSalary(true, plain, "newest", { ...EMPTY, salaryMax: "12000" })).toBe(true);
  });

  it("never for those who may not see salaries", () => {
    expect(wantsSalary(false, [...plain, "expectedSalary"], "expected_salary_asc", { ...EMPTY, salaryMax: "12000" })).toBe(false);
  });
});
