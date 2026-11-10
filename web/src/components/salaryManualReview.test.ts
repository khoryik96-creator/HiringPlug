import { expect, it } from "vitest";
import { formFromCompensation, salaryProblems } from "./salaryFormState";

it("rejects backwards ranges and amounts outside the stored money limit", () => {
  const form = {...formFromCompensation(null),currentAmount:"1000000000000",expectedMin:"13000",expectedMax:"12000"};
  expect(salaryProblems(form)).toMatchObject({"currentSalary.amount":"The amount is outside the supported range.","expectedSalary.max":"The upper figure must be at least the lower figure."});
  expect(salaryProblems({...form,currentAmount:"999999999999.99",expectedMin:"0",expectedMax:"0"})).toEqual({});
});
