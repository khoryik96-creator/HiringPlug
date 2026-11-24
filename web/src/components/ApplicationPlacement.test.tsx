import { cleanup, screen, waitFor } from "@testing-library/react";
import { focusManager } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, RECRUITER, renderWithClient } from "../test-utils";
import { ApplicationPlacement } from "./ApplicationPlacement";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

const placement = (deliveryStatus: string, salary: boolean): Schemas["Placement"] => ({
  id: "plc_1", version: 1, applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah", jobId: "jo_1", jobTitle: "Finance Manager",
  companyId: "comp_1", companyName: "Maju Foods", jobType: "permanent", poNumber: null, endDate: null, recruiterId: null, recruiterName: null,
  startDate: "2026-12-01", guaranteeDays: 60, guaranteeEndsOn: "2027-01-30", status: "active", leftOn: null, statusNote: null, replacement: "none",
  invoiceStatus: deliveryStatus === "queued" ? "not_invoiced" : "invoiced", invoiceNumber: deliveryStatus === "queued" ? null : "INV-0042",
  invoicedOn: null, paidOn: null, restrictedFields: [], createdAt: "2026-11-09T00:00:00Z", updatedAt: "2026-11-09T00:00:00Z",
  money: { salary: salary ? { amount: "7345.00", currency: "MYR", period: "month" } : null, feePercent: "20", feeAmount: "17628.00", feeCurrency: "MYR" },
  invoicing: { contactId: "cont_ken", contactName: "Ken Lim", contactEmail: "ken@maju.test", paymentTermsDays: 30, dueOn: "2026-12-31",
    dueOnSet: false, autoSend: true, sendsAt: null, sentAt: null, sendError: null, deliveryStatus },
} as never);

describe("an application's placement", () => {
  it("watches a queued invoice without reading the salary again, and keeps the salary shown", async () => {
    let sent = false;
    const get = vi.spyOn(api, "GET").mockImplementation(((_: string, init: { params: { query: { includeSalary?: boolean } } }) => {
      const status = sent ? "sent" : "queued";
      return Promise.resolve(ok({ items: [placement(status, init.params.query.includeSalary === true)] }));
    }) as never);
    const asked = () => get.mock.calls.map((c) => (c[1] as { params: { query: { includeSalary?: boolean } } }).params.query.includeSalary === true);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderWithClient(<ApplicationPlacement applicationId="appl_1" stage="placed" permissions={RECRUITER.concat("placements.fees:read")} />);
    expect(await screen.findByText("MYR 7,345.00 / month")).toBeTruthy();
    await waitFor(() => { expect(asked()).toEqual([true, false]); }); // the salary once, then where the invoice stands
    sent = true;
    await vi.advanceTimersByTimeAsync(5_100);
    await waitFor(() => { expect(screen.getAllByText(/INV-0042/).length).toBeGreaterThan(0); }); // where it stands, read again
    expect(asked().filter(Boolean)).toHaveLength(1);
    expect(screen.getByText("MYR 7,345.00 / month")).toBeTruthy(); // the salary detail, kept from the first read
    await vi.advanceTimersByTimeAsync(10_000);
    const polls = asked().length;
    // Back to the window: the salary is not read again (each read is counted as shown).
    focusManager.setFocused(false);
    focusManager.setFocused(true);
    await vi.advanceTimersByTimeAsync(100);
    expect(asked().length).toBe(polls);
    focusManager.setFocused(undefined);
  });
});
