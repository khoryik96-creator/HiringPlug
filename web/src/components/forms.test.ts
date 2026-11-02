import { describe, expect, it } from "vitest";

import { ApiError } from "../api/client";
import { countryName, formatNotice } from "../labels";
import { testCandidate } from "../test-utils";
import { candidateProblems, formFromCandidate, toUpdateBody } from "./candidateFormState";
import { fieldErrors, FormInvalid } from "./forms";
import { formFromCompensation, plainAmount, salaryProblems, toCompensationBody } from "./salaryFormState";

const candidate = testCandidate();

describe("candidate form state", () => {
  it("sends every field with the given version; empty inputs clear fields", () => {
    const form = { ...formFromCandidate(candidate), email: "  ", nationality: "sg", currentTitle: "Senior Accountant" };
    const body = toUpdateBody(form, candidate.version);
    expect(body.version).toBe(3);
    expect(body.email).toBeNull();
    expect(body.nationality).toBe("SG");
    expect(body.currentTitle).toBe("Senior Accountant");
    expect(body.noticePeriod).toEqual({ value: 1, unit: "month" });
    expect(body.workAuthorization).toBeNull();
  });

  it("only sends a pass type for expats who can already work here", () => {
    const local = toUpdateBody({ ...formFromCandidate(candidate), waStatus: "local", waBasis: "rp_t" }, 1);
    expect(local.workAuthorization).toEqual({ status: "local", basis: null, note: null });
    const expat = toUpdateBody({ ...formFromCandidate(candidate), waStatus: "expat_eligible", waBasis: "rp_t" }, 1);
    expect(expat.workAuthorization?.basis).toBe("rp_t");
  });

  it("wants a 2-letter nationality code, in any case", () => {
    const form = formFromCandidate(candidate);
    expect(candidateProblems({ ...form, nationality: "my" })).toEqual({});
    expect(candidateProblems({ ...form, nationality: "" })).toEqual({});
    expect(Object.keys(candidateProblems({ ...form, nationality: "Malaysian" }))).toEqual(["nationality"]);
  });
});

describe("salary form state", () => {
  it("strips thousands separators and leaves out empty figures", () => {
    const form = { ...formFromCompensation(null), currentAmount: "8,500", expectedMin: "10 000", expectedMax: "" };
    expect(toCompensationBody(form, 0)).toEqual({
      expectedVersion: 0,
      currentSalary: { amount: "8500", currency: "MYR", period: "month" },
      expectedSalary: { min: "10000", max: null, currency: "MYR", period: "month" },
    });
    expect(toCompensationBody(formFromCompensation(null), 2).currentSalary).toBeNull();
  });

  it("starts a figure not recorded yet with the other figure's currency and period", () => {
    const yearly = { amount: "180000.00", currency: "SGD", period: "year" } as const;
    const onlyCurrent = formFromCompensation({ version: 1, updatedAt: "", currentSalary: yearly, expectedSalary: null });
    expect([onlyCurrent.expectedCurrency, onlyCurrent.expectedPeriod]).toEqual(["SGD", "year"]);
    const range = { min: "9000.00", max: null, currency: "SGD", period: "month" } as const;
    const onlyExpected = formFromCompensation({ version: 1, updatedAt: "", currentSalary: null, expectedSalary: range });
    expect([onlyExpected.currentCurrency, onlyExpected.currentPeriod]).toEqual(["SGD", "month"]);
  });

  it("reads commas as thousands separators only when they are", () => {
    expect(plainAmount("8,500")).toBe("8500");
    expect(plainAmount("1,234,567.89")).toBe("1234567.89");
    expect(plainAmount("1,00,000")).toBe("100000"); // Indian grouping
    expect(plainAmount(" 10 000 ")).toBe("10000");
    // A decimal comma is left alone, to be refused, never read as 850050 or 4550.
    expect(plainAmount("8500,50")).toBe("8500,50");
    expect(plainAmount("45,50")).toBe("45,50");
  });

  it("finds amounts, a lone To and currencies it cannot send", () => {
    const blank = formFromCompensation(null);
    expect(salaryProblems({ ...blank, currentAmount: "8,500.50", expectedMin: "9000", expectedMax: "11,000" })).toEqual({});
    expect(Object.keys(salaryProblems({ ...blank, currentAmount: "8500,50" }))).toEqual(["currentSalary.amount"]);
    expect(Object.keys(salaryProblems({ ...blank, expectedMax: "12000" }))).toEqual(["expectedSalary.min"]);
    expect(Object.keys(salaryProblems({ ...blank, currentAmount: "8500", currentCurrency: "RM" }))).toEqual(["currentSalary.currency"]);
    // A currency only matters when its figure is filled in.
    expect(salaryProblems({ ...blank, currentCurrency: "" })).toEqual({});
  });
});

describe("field errors", () => {
  it("maps VALIDATION_FAILED details to fields", () => {
    const err = new ApiError({
      error: {
        code: "VALIDATION_FAILED", message: "Some fields are invalid.", requestId: "req_1",
        details: { fields: [{ field: "workAuthorization.basis", message: "is required" }] },
      },
    });
    expect(fieldErrors(err)).toEqual({ "workAuthorization.basis": "is required" });
    expect(fieldErrors(new FormInvalid({ nationality: "Use the code" }))).toEqual({ nationality: "Use the code" });
    expect(fieldErrors(new Error("x"))).toEqual({});
  });
});

describe("labels", () => {
  it("names countries and notice periods", () => {
    expect(countryName("MY")).toBe("Malaysia (MY)");
    expect(countryName("JA")).toBe("JA"); // not a country: shown as stored
    expect(formatNotice({ value: 0, unit: "month" })).toBe("Immediate");
    expect(formatNotice({ value: 1, unit: "week" })).toBe("1 week");
    expect(formatNotice({ value: 3, unit: "month" })).toBe("3 months");
    expect(formatNotice(null)).toBe("—");
  });
});
