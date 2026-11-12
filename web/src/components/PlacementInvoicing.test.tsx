import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import * as files from "../api/files";
import { invalidField, ok, RECRUITER, renderWithClient, sentBody } from "../test-utils";
import { PlacementForm } from "./PlacementForm";
import { PlacementInvoice } from "./PlacementInvoicing";
import { autoDueOn } from "./placementFormState";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const contacts = [
  { id: "cont_aisyah", fullName: "Aisyah Rahman", email: "aisyah@maju.test" },
  { id: "cont_ken", fullName: "Ken Lim", email: "ken@maju.test" },
  { id: "cont_wei", fullName: "Wei Tan", email: null },
];
const rate = (salaryFrom: string | null, salaryBelow: string | null, feePercent: string, guaranteeDays: number, paymentTermsDays: number, remarks: string | null = null) =>
  ({ salaryFrom, salaryBelow, feePercent, guaranteeDays, paymentTermsDays, remarks });
const defaults: Schemas["PlacementDefaults"] = {
  companyId: "comp_1", companyName: "Maju Foods", jobTitle: "Finance Manager", jobType: "contract", termsSet: true, feePercent: "18", paymentTermsDays: 30, guaranteeDays: 60, contactId: "cont_aisyah",
  tiers: [rate(null, null, "18", 60, 30)], salaryCurrency: "MYR",
};
const placement = (over: Partial<Schemas["Placement"]> = {}): Schemas["Placement"] => ({
  id: "plc_1", version: 1, applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah", jobId: "jo_1", jobTitle: "Finance Manager",
  companyId: "comp_1", companyName: "Maju Foods", jobType: "permanent", poNumber: null, endDate: null, recruiterId: null, recruiterName: null, startDate: "2026-12-01", guaranteeDays: 60,
  guaranteeEndsOn: "2027-01-30", status: "active", leftOn: null, statusNote: null, replacement: "none", invoiceStatus: "not_invoiced",
  invoiceNumber: null, invoicedOn: null, paidOn: null, money: null, restrictedFields: [], createdAt: "2026-11-09T00:00:00Z", updatedAt: "2026-11-09T00:00:00Z",
  invoicing: { contactId: "cont_ken", contactName: "Ken Lim", contactEmail: "ken@maju.test", paymentTermsDays: 30, dueOn: "2026-12-31",
    dueOnSet: false, autoSend: true, sendsAt: "2026-12-01T01:00:00Z", sentAt: null, sendError: null },
  ...over,
});

describe("invoicing a placement", () => {
  it("starts from the client's terms, invoices whom you choose, and emails it on the start date", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/applications/{applicationId}/placement-defaults" ? defaults : path === "/v1/contacts" ? { items: contacts, nextCursor: null } : { items: [] },
    ))) as never);
    vi.spyOn(api, "POST").mockResolvedValue(ok(placement()));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(placement()));
    renderWithClient(<PlacementForm applicationId="appl_1" permissions={RECRUITER.concat("placements.fees:update")} onDone={() => undefined} />);

    expect(await screen.findByText(/From Maju Foods's terms of business: 18% fee · 30 days to pay · 2 months guarantee/)).toBeTruthy();
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Guarantee (days)").value).toBe("60"); });
    expect(screen.getByLabelText<HTMLInputElement>("Fee %").value).toBe("18");
    const to = screen.getByLabelText<HTMLSelectElement>("Send the invoice to");
    expect(to.value).toBe("cont_aisyah"); // the hiring contact
    expect(within(to).getByRole("option", { name: "Wei Tan — no email address" })).toHaveProperty("disabled", true);
    fireEvent.change(to, { target: { value: "cont_ken" } });
    fireEvent.change(screen.getByLabelText("Start date"), { target: { value: "2026-12-01" } });
    expect(screen.getByLabelText<HTMLInputElement>("Payment due").value).toBe("2026-12-31"); // start + 30 days
    fireEvent.change(screen.getByLabelText("Payment terms (days)"), { target: { value: "45" } });
    expect(screen.getByLabelText<HTMLInputElement>("Payment due").value).toBe("2027-01-15");
    expect(screen.getByLabelText<HTMLInputElement>(/Email the invoice to Ken Lim on the start date/).checked).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Record placement" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ contactId: "cont_ken", paymentTermsDays: 45, dueOn: null, autoSend: true });
  });

  it("takes the rate for the salary from tiered terms", async () => {
    const tiered = { ...defaults, feePercent: null, tiers: [rate(null, "120000", "18", 60, 14), rate("120000", null, "25", 90, 30, "50% on signing")] };
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/applications/{applicationId}/placement-defaults" ? tiered : path === "/v1/contacts" ? { items: contacts, nextCursor: null } : { items: [] },
    ))) as never);
    renderWithClient(<PlacementForm applicationId="appl_1" permissions={RECRUITER.concat("placements.fees:update")} onDone={() => undefined} />);
    expect(await screen.findByText(/rates depend on the annual salary \(Below RM 120,000: 18%; RM 120,000 and above: 25%\)/)).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Agreed salary"), { target: { value: "12,000" } }); // a month: RM 144,000 a year
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Fee %").value).toBe("25"); });
    expect(screen.getByLabelText<HTMLInputElement>("Guarantee (days)").value).toBe("90");
    expect(screen.getByLabelText<HTMLInputElement>("Payment terms (days)").value).toBe("30");
    expect(screen.getByRole("note").textContent).toContain("(RM 120,000 and above): 25% fee · 30 days to pay · 3 months guarantee. Payment: 50% on signing.");

    fireEvent.change(screen.getByLabelText("Fee %"), { target: { value: "22" } }); // changed for this placement: kept
    fireEvent.change(screen.getByLabelText("Agreed salary"), { target: { value: "12,500" } });
    expect(screen.getByLabelText<HTMLInputElement>("Fee %").value).toBe("22");
    fireEvent.change(screen.getByLabelText("Agreed salary"), { target: { value: "9000" } }); // into the lower band
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Fee %").value).toBe("18"); });
    expect(screen.getByLabelText<HTMLInputElement>("Payment terms (days)").value).toBe("14");
  });

  it("shows where the invoice stands, previews it and sends it now", async () => {
    const download = vi.spyOn(files, "downloadFrom").mockResolvedValue();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(placement()));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<PlacementInvoice p={placement()} permissions={RECRUITER.concat("placements.fees:read", "placements:manage")} />);
    expect(screen.getByText(/Emails to Ken Lim on 1 Dec 2026/)).toBeTruthy();
    const fetched = vi.spyOn(files, "fetchFile").mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
    URL.createObjectURL = vi.fn(() => "blob:invoice");
    URL.revokeObjectURL = vi.fn();
    fireEvent.click(screen.getByRole("button", { name: "Preview invoice" }));
    const viewer = await screen.findByTitle("Invoice preview: Siti Aisyah (numbered when sent)");
    expect(viewer.getAttribute("src")).toMatch(/^blob:invoice#/); // shown in the app, not downloaded
    expect(fetched).toHaveBeenCalledWith("/v1/placements/plc_1/invoice/file?format=pdf", "application/pdf");
    expect(download).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Download PDF" }));
    expect(download).toHaveBeenCalledWith("/v1/placements/plc_1/invoice/file?format=pdf", "Invoice Siti Aisyah.pdf", "application/pdf");
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByTitle("Invoice preview: Siti Aisyah (numbered when sent)")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Send now" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    cleanup();

    renderWithClient(<PlacementInvoice p={placement({ invoiceStatus: "invoiced", invoiceNumber: "HYP-0042",
      invoicing: { ...placement().invoicing, sentAt: "2026-12-01T01:00:00Z", sendsAt: null } })} permissions={RECRUITER.concat("placements.fees:read")} />);
    expect(screen.getByText(/Emailed to Ken Lim on 1 Dec 2026/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Send now" })).toBeNull();
    cleanup();

    renderWithClient(<PlacementInvoice p={placement({ invoicing: { ...placement().invoicing, sendsAt: null, sendError: "Record the placement's fee first." } })}
      permissions={RECRUITER} />);
    expect(screen.getByRole("alert").textContent).toContain("Record the placement's fee first.");
    expect(screen.queryByRole("button", { name: "Preview invoice" }), "the invoice shows the fee").toBeNull();
  });

  it("a failed invoicing save does not record the placement twice", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/applications/{applicationId}/placement-defaults" ? defaults : path === "/v1/contacts" ? { items: contacts, nextCursor: null } : { items: [] },
    ))) as never);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(placement()));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(placement({ version: 2 })));
    const put = vi.spyOn(api, "PUT").mockResolvedValueOnce(invalidField("paymentTermsDays", "between 0 and 365 days")).mockResolvedValue(ok(placement()));
    renderWithClient(<PlacementForm applicationId="appl_1" permissions={RECRUITER} onDone={() => undefined} />);
    await screen.findByText(/terms of business/);
    fireEvent.change(screen.getByLabelText("Start date"), { target: { value: "2026-12-01" } });
    fireEvent.click(screen.getByRole("button", { name: "Record placement" }));
    expect(await screen.findByText("between 0 and 365 days")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Record placement" }));
    await waitFor(() => { expect(put).toHaveBeenCalledTimes(2); });
    expect(post).toHaveBeenCalledTimes(1);
    expect(patch).toHaveBeenCalledTimes(1); // the second save changes the one recorded
  });

  it("works out the due date", () => {
    expect(autoDueOn("2026-12-01", "30")).toBe("2026-12-31");
    expect(autoDueOn("2027-01-31", "30")).toBe("2027-03-02");
    expect(autoDueOn("", "30")).toBe("");
  });
});
