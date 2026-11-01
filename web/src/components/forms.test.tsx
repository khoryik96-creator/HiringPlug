import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ApiError, type Candidate } from "../api/client";
import { CandidateForm } from "./CandidateForm";
import { formFromCandidate, toUpdateBody } from "./candidateForm";
import { fieldErrors } from "./forms";
import { formFromCompensation, toCompensationBody } from "./salaryForm";

const candidate: Candidate = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 3, firstName: "Siti", lastName: "Aisyah", fullName: "Siti Aisyah",
  email: "siti@example.com", phone: null, nationality: "MY", workAuthorization: null,
  noticePeriod: { value: 1, unit: "month" }, industry: null, specialization: null, currentTitle: "Accountant",
  currentCompany: null, location: null, source: "jobstreet", restrictedFields: [],
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

describe("candidate form", () => {
  it("sends every field with the loaded version; empty inputs clear fields", () => {
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

  it("asks which pass an expat holds only when it applies", () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <CandidateForm candidate={candidate} onDone={() => undefined} />
      </QueryClientProvider>,
    );
    expect(screen.queryByText("Pass held")).toBeNull();
    fireEvent.change(screen.getByLabelText("Work authorization"), { target: { value: "expat_eligible" } });
    expect(screen.getByText("Pass held")).toBeTruthy();
  });
});

describe("salary form", () => {
  it("strips thousands separators and leaves out empty figures", () => {
    const form = { ...formFromCompensation(null), currentAmount: "8,500", expectedMin: "10 000", expectedMax: "" };
    expect(toCompensationBody(form, 0)).toEqual({
      expectedVersion: 0,
      currentSalary: { amount: "8500", currency: "MYR", period: "month" },
      expectedSalary: { min: "10000", max: null, currency: "MYR", period: "month" },
    });
    expect(toCompensationBody(formFromCompensation(null), 2).currentSalary).toBeNull();
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
    expect(fieldErrors(new Error("x"))).toEqual({});
  });
});
