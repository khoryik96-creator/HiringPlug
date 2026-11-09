import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { CompanyTerms } from "./CompanyTerms";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const none = { feePercent: null, paymentTermsDays: null, guaranteeDays: null, signedOn: null, notes: null, version: 0, updatedAt: null };
const signed = { feePercent: "18", paymentTermsDays: 30, guaranteeDays: 60, signedOn: "2026-01-15", notes: "One free replacement.", version: 1, updatedAt: "2026-11-09T03:00:00Z" };

describe("a client's terms of business", () => {
  it("are added: fee, payment terms, guarantee, when signed", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(none));
    const put = vi.spyOn(api, "PUT").mockResolvedValueOnce(invalidField("feePercent", "a percentage above 0, at most 100")).mockResolvedValue(ok(signed));
    renderWithClient(<CompanyTerms companyId="comp_1" permissions={["companies:read", "companies:update"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Add terms" }));
    fireEvent.change(screen.getByLabelText("Fee (% of annual salary)"), { target: { value: "18%" } });
    fireEvent.change(screen.getByLabelText("Guarantee (days)"), { target: { value: "60" } });
    fireEvent.change(screen.getByLabelText("Signed on"), { target: { value: "2026-01-15" } });
    fireEvent.click(screen.getByRole("button", { name: "Save terms" }));
    expect(await screen.findByText("a percentage above 0, at most 100")).toBeTruthy();
    expect(sentBody(put)).toEqual({ version: 0, feePercent: "18", paymentTermsDays: 30, guaranteeDays: 60, signedOn: "2026-01-15", notes: null });
    fireEvent.click(screen.getByRole("button", { name: "Save terms" }));
    await waitFor(() => { expect(screen.getByText("18% of annual salary")).toBeTruthy(); });
    expect(screen.getByText("30 days")).toBeTruthy();
    expect(screen.getByText("One free replacement.")).toBeTruthy();
  });

  it("are read only without companies:update", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(signed));
    renderWithClient(<CompanyTerms companyId="comp_1" permissions={["companies:read"]} />);
    expect(await screen.findByText("60 days")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });
});
