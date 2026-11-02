import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ApiError, type Candidate, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { SalaryPanel } from "./SalaryPanel";

const base: Candidate = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, firstName: "Siti", lastName: "Aisyah", fullName: "Siti Aisyah",
  email: null, phone: null, nationality: "MY", workAuthorization: { status: "local" }, noticePeriod: null,
  industry: null, specialization: null, currentTitle: null, currentCompany: null, location: null, source: null,
  tags: [], skills: [], restrictedFields: [], createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

describe("SalaryPanel", () => {
  it("tells restricted roles that salary is hidden, without showing any figure", () => {
    render(<SalaryPanel candidate={{ ...base, restrictedFields: ["compensation"] }} />);
    expect(screen.getByText(/restricted for your role/)).toBeTruthy();
  });

  it("shows current and expected salary with currency and period", () => {
    render(
      <SalaryPanel
        candidate={{
          ...base,
          compensation: {
            version: 1, updatedAt: "2026-11-01T00:00:00Z",
            currentSalary: { amount: "8500.00", currency: "MYR", period: "month" },
            expectedSalary: { min: "10000.00", max: "11000.00", currency: "MYR", period: "month" },
          },
        }}
      />,
    );
    expect(screen.getByText("MYR 8500.00 / month")).toBeTruthy();
    expect(screen.getByText("MYR 10000.00 – 11000.00 / month")).toBeTruthy();
  });
});

describe("errors", () => {
  const body = { error: { code: "CONFLICT_STALE_VERSION" as const, message: "Changed by someone else.", requestId: "req_abc" } };

  it("unwrap turns an error body into an ApiError with code and request ID", () => {
    const response = new Response(null, { status: 409 });
    expect(() => unwrap({ error: body, response })).toThrow(ApiError);
  });

  it("shows the request ID so support can find the logs", () => {
    render(<ErrorMessage error={new ApiError(body)} />);
    expect(screen.getByText("req_abc")).toBeTruthy();
  });
});
