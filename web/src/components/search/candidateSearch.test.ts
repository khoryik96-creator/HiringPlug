import { describe, expect, it } from "vitest";

import { wantsSalary } from "./candidateSearch";

describe("wantsSalary", () => {
  const plain = ["currentRole", "status", "email"];

  it("asks for salaries only when the list shows them", () => {
    expect(wantsSalary(true, [...plain, "expectedSalary"], false)).toBe(true);
    expect(wantsSalary(true, [...plain, "currentSalary"], false)).toBe(true);
    expect(wantsSalary(true, plain, false)).toBe(false);
  });

  it("asks in the full-resume reader, which shows each candidate's pay", () => {
    expect(wantsSalary(true, plain, true)).toBe(true);
  });

  it("never for those who may not see salaries", () => {
    expect(wantsSalary(false, [...plain, "expectedSalary"], true)).toBe(false);
  });
});
