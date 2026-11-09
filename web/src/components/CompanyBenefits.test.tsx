import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CompanyBenefits } from "./CompanyBenefits";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const empty = { benefits: [], preOffer: [], version: 0, updatedAt: null };
const saved = {
  version: 2, updatedAt: "2026-11-09T03:00:00Z",
  benefits: [
    { key: "annual_leave", label: "Annual leave", value: "18 days", remarks: "21 after 3 years" },
    { key: "dental_optical", label: "Dental & optical", value: null, remarks: null },
    { key: "car", label: "Car allowance", value: "RM 800", remarks: null }, // since taken out of the template
  ],
  preOffer: [{ key: "reference_check", label: "Reference check", applies: true, remarks: "2 referees" }],
};
const answer = (benefits: unknown) => vi.spyOn(api, "GET").mockImplementation((path: string) =>
  Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok(benefits)));

describe("a client's benefits", () => {
  it("is filled in from the template, item by item, with what to write under each", async () => {
    answer(empty);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(saved));
    renderWithClient(<CompanyBenefits companyId="comp_1" companyName="Maju Foods" permissions={["companies:read", "companies:update"]} />);
    expect(await screen.findByText("Nothing recorded yet.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Fill in" }));

    const benefits = screen.getByRole("group", { name: "Benefits" });
    expect(benefits.textContent).toContain("If it grows with service or grade, give the detail");
    fireEvent.change(within(benefits).getByLabelText("Annual leave: value"), { target: { value: "18 days" } });
    fireEvent.change(within(benefits).getByLabelText("Fixed bonus: remarks"), { target: { value: "13th month, paid in December" } });
    const steps = screen.getByRole("group", { name: "Pre-offer process" });
    fireEvent.click(within(within(steps).getByRole("radiogroup", { name: "Reference check: applies" })).getByRole("radio", { name: "Yes" }));
    fireEvent.click(within(within(steps).getByRole("radiogroup", { name: "Health screening: applies" })).getByRole("radio", { name: "No" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({
      version: 0,
      benefits: [
        { key: "annual_leave", label: "Annual leave", value: "18 days", remarks: null },
        { key: "dental_optical", label: "Dental & optical", value: null, remarks: null },
        { key: "fixed_bonus", label: "Fixed bonus", value: null, remarks: "13th month, paid in December" },
      ],
      preOffer: [
        { key: "reference_check", label: "Reference check", applies: true, remarks: null },
        { key: "health_screening", label: "Health screening", applies: false, remarks: null },
      ],
    });
  });

  it("reads at a glance: what is recorded, what is not yet, and each step's answer", async () => {
    answer(saved);
    renderWithClient(<CompanyBenefits companyId="comp_1" companyName="Maju Foods" permissions={["companies:read"]} />);
    expect(await screen.findByText("18 days")).toBeTruthy();
    expect(screen.getByText("21 after 3 years")).toBeTruthy();
    expect(screen.getByText(/Car allowance/).textContent).toContain("(not in the template)");
    expect(screen.getByText(/Not recorded yet:/).textContent).toBe("Not recorded yet: Dental & optical · Fixed bonus");
    expect(screen.getByText(/2 of 4 benefits recorded/)).toBeTruthy();
    const steps = screen.getByRole("list");
    expect(steps.textContent).toContain("YesReference check2 referees");
    expect(steps.textContent).toContain("Not knownHealth screening");
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.getByRole("button", { name: "Word" })).toBeTruthy();
  });
});
