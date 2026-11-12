import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ApplicationPlacement } from "../components/ApplicationPlacement";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { daysUntil, Placements } from "./Placements";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.removeItem("hiringplug.placements.view");
});

const placement = (over: Partial<Schemas["Placement"]> = {}): Schemas["Placement"] => ({
  id: "plc_1", version: 1, applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah", jobId: "jo_1",
  jobTitle: "Accountant", companyId: "comp_1", companyName: "Contoh Berhad", jobType: "permanent", poNumber: null, endDate: null,
  invoicing: { contactId: "cont_1", contactName: "Mei Tan", contactEmail: "mei@contoh.test", paymentTermsDays: 30, dueOn: "2027-01-04",
    dueOnSet: false, autoSend: true, sendsAt: null, sentAt: "2026-12-05T01:00:00Z", sendError: null }, recruiterId: "usr_1", recruiterName: "Aina",
  startDate: "2026-12-01", guaranteeDays: 90, guaranteeEndsOn: "2027-03-01", status: "active", leftOn: null, statusNote: null, replacement: "none",
  invoiceStatus: "invoiced", invoiceNumber: "INV-1", invoicedOn: "2026-12-05", paidOn: null,
  money: { salary: { amount: "8500.00", currency: "MYR", period: "month" }, feePercent: "20.00", feeAmount: "20400.00", feeCurrency: "MYR", retainer: null },
  restrictedFields: [], createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  ...over,
});
const noTerms: Schemas["PlacementDefaults"] = {
  companyId: "comp_1", companyName: "Maju Foods", jobTitle: "Finance Manager", jobType: "permanent", termsSet: false, tiers: [], salaryCurrency: "MYR", feePercent: null,
  paymentTermsDays: 30, guaranteeDays: 90, contactId: null,
};
const reads = (list: Schemas["PlacementList"]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/users") return Promise.resolve(ok({ items: [{ id: "usr_1", name: "Aina", role: "recruiter", canInterview: true }] }));
    if (path === "/v1/applications/{applicationId}/placement-defaults") return Promise.resolve(ok(noTerms));
    return Promise.resolve(ok(list));
  }) as never);

describe("placements", () => {
  it("lists placements with fee totals", async () => {
    reads({ items: [placement()], total: 1, today: "2026-12-10", feeTotals: [{ currency: "MYR", total: "20400.00", notInvoiced: "0.00", invoiced: "20400.00", paid: "0.00" }] });
    renderWithClient(<Placements permissions={["placements:read"]} />);
    expect(await screen.findByText("Siti Aisyah")).toBeTruthy();
    const row = screen.getByRole("row", { name: /Siti Aisyah/ });
    expect(row.textContent, "no fee without placements.fees:read").not.toContain("MYR 20,400.00");
    expect(row.textContent).toContain("Permanent");
    expect(row.textContent).toContain("Invoiced");
    expect(screen.queryByRole("button", { name: "Change" })).toBeNull();
    expect(screen.queryByText("⋯"), "nothing to do: no menu").toBeNull();
  });

  it("opens a placement's form from its menu", async () => {
    reads({ items: [placement({ poNumber: "PO-1", endDate: "2027-05-31", jobType: "contract" })], total: 1, today: "2026-12-10", feeTotals: [] });
    renderWithClient(<Placements permissions={["placements:read", "placements:manage", "placements.fees:read"]} />);
    const row = await screen.findByRole("row", { name: /Siti Aisyah/ });
    expect(row.textContent).toContain("PO-1");
    expect(row.textContent).toContain("to 31 May 2027");
    fireEvent.click(screen.getByText("⋯"));
    fireEvent.click(screen.getByRole("button", { name: "Change" }));
    expect(await screen.findByRole("form", { name: "Change placement" })).toBeTruthy();
  });

  it("records a placement with its fee, from an offer", async () => {
    reads({ items: [], total: 0, today: "2026-12-10" });
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
    const get = reads({ items: [], total: 0, today: "2026-12-10" });
    renderWithClient(<ApplicationPlacement applicationId="appl_1" stage="interview" permissions={["placements:read", "placements:manage"]} />);
    expect(screen.queryByRole("button", { name: "Record placement" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("counts and filters with the cards, on the agency's day, and shows each guarantee's days left", async () => {
    const get = reads({
      today: "2026-12-10", total: 4,
      items: [
        placement({ id: "plc_1", candidateName: "Ending Soon", startDate: "2026-09-20", guaranteeEndsOn: "2026-12-19", invoiceStatus: "paid", paidOn: "2026-10-01" }),
        placement({ id: "plc_2", candidateName: "Starts Later", startDate: "2027-01-05", guaranteeEndsOn: "2027-04-05", invoiceStatus: "not_invoiced", invoicedOn: null, invoiceNumber: null }),
        placement({ id: "plc_3", candidateName: "Long Overdue", startDate: "2026-10-01", guaranteeEndsOn: "2026-12-30", invoicing: { ...placement().invoicing, dueOn: "2026-11-01" } }),
        placement({ id: "plc_4", candidateName: "Left Early", status: "fell_through", leftOn: "2026-11-01", replacement: "owed" }),
      ],
      feeTotals: [{ currency: "MYR", total: "61200.00", notInvoiced: "20400.00", invoiced: "20400.00", paid: "20400.00" }],
    });
    renderWithClient(<Placements permissions={["placements:read", "placements.fees:read"]} />);
    const cards = await screen.findByRole("group", { name: "Show" });
    const card = (name: RegExp) => within(cards).getByRole("button", { name });
    expect(card(/^4\s*Placements/).textContent).toContain("4");
    expect(card(/Starting soon/).textContent).toMatch(/^1/);
    expect(card(/In guarantee/).textContent).toMatch(/^2/);
    expect(card(/Guarantee ending/).textContent).toMatch(/^2/);
    expect(card(/Not invoiced/).textContent).toMatch(/^1/);
    expect(card(/Overdue/).textContent).toMatch(/^1/);
    expect(card(/Replacement owed/).textContent).toMatch(/^1/);
    expect(screen.getByRole("region", { name: "Fees" }).textContent).toContain("MYR 61,200.00");

    expect(screen.getByRole("row", { name: /Ending Soon/ }).textContent).toContain("9 days left");
    expect(screen.getByRole("row", { name: /Starts Later/ }).textContent).toContain("starts in 26 days");
    fireEvent.click(card(/Overdue/));
    expect(screen.queryByRole("row", { name: /Ending Soon/ })).toBeNull();
    expect(screen.getByRole("row", { name: /Long Overdue/ })).toBeTruthy();
    fireEvent.click(card(/Overdue/));
    expect(screen.getByRole("row", { name: /Ending Soon/ }), "clicking the card again shows all").toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "All time" }));
    const lastQuery = () => {
      const calls = get.mock.calls.map((c) => c as unknown[]).filter((c) => c[0] === "/v1/placements");
      return calls.at(-1)?.[1];
    };
    await waitFor(() => { expect(lastQuery()).toEqual({ params: { query: {} } }); });
  });

  it("says when the list stopped short, instead of passing it off as all of them", async () => {
    reads({ items: [placement()], total: 730, today: "2026-12-10" });
    renderWithClient(<Placements permissions={["placements:read"]} />);
    expect(await screen.findByText(/Only the newest 1 of 730 placements in these dates are loaded/)).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Fees" }), "no fees without placements.fees:read").toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Fee" })).toBeNull();
  });

  it("counts days to a date", () => {
    expect(daysUntil("2026-11-17", new Date(2026, 10, 3))).toBe(14);
    expect(daysUntil("2026-11-01", new Date(2026, 10, 3))).toBe(-2);
  });
});
