import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ApplicationPlacement } from "../components/ApplicationPlacement";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { daysUntil, Placements } from "./Placements";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const placement = (over: Partial<Schemas["Placement"]> = {}): Schemas["Placement"] => ({
  id: "plc_1", version: 1, applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah", jobId: "jo_1",
  jobTitle: "Accountant", companyName: "Contoh Berhad", recruiterId: "usr_1", recruiterName: "Aina",
  startDate: "2026-12-01", guaranteeDays: 90, guaranteeEndsOn: "2027-03-01", status: "active", leftOn: null, statusNote: null, replacement: "none",
  invoiceStatus: "invoiced", invoiceNumber: "INV-1", invoicedOn: "2026-12-05", paidOn: null,
  money: { salary: { amount: "8500.00", currency: "MYR", period: "month" }, feePercent: "20.00", feeAmount: "20400.00", feeCurrency: "MYR" },
  restrictedFields: [], createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  ...over,
});
const reads = (list: Schemas["PlacementList"]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/users") return Promise.resolve(ok({ items: [{ id: "usr_1", name: "Aina", role: "recruiter", canInterview: true }] }));
    return Promise.resolve(ok(list));
  }) as never);

describe("placements", () => {
  it("lists placements with fee totals", async () => {
    reads({ items: [placement()], feeTotals: [{ currency: "MYR", total: "20400.00", notInvoiced: "0.00", invoiced: "20400.00", paid: "0.00" }] });
    renderWithClient(<Placements permissions={["placements:read"]} />);
    expect(await screen.findByText("Siti Aisyah")).toBeTruthy();
    expect(screen.getByText(/fee MYR 20,400.00/)).toBeTruthy();
    expect(screen.getAllByText("MYR 20,400.00").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Change" })).toBeNull();
  });

  it("records a placement with its fee, from an offer", async () => {
    reads({ items: [] });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(placement()));
    renderWithClient(<ApplicationPlacement applicationId="appl_1" stage="offer" permissions={["placements:read", "placements:manage", "placements.fees:update"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Record placement" }));
    fireEvent.change(screen.getByLabelText("Start date"), { target: { value: "2026-12-01" } });
    fireEvent.change(screen.getByLabelText("Agreed salary"), { target: { value: "8,500" } });
    fireEvent.change(screen.getByLabelText("Fee %"), { target: { value: "20" } });
    expect(screen.getByRole("status").textContent).toContain("MYR 20,400.00");
    fireEvent.click(screen.getByRole("button", { name: "Record placement" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({
      startDate: "2026-12-01", guaranteeDays: 90,
      money: { salary: { amount: "8500", currency: "MYR", period: "month" }, feePercent: "20", feeAmount: null, feeCurrency: null },
    });
  });

  it("shows nothing before the offer stage", () => {
    const get = reads({ items: [] });
    renderWithClient(<ApplicationPlacement applicationId="appl_1" stage="interview" permissions={["placements:read", "placements:manage"]} />);
    expect(screen.queryByRole("button", { name: "Record placement" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("counts days to a date", () => {
    expect(daysUntil("2026-11-17", new Date(2026, 10, 3))).toBe(14);
    expect(daysUntil("2026-11-01", new Date(2026, 10, 3))).toBe(-2);
  });
});
