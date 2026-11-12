import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { JobRetainer } from "./JobRetainer";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const none: Schemas["JobRetainer"] = { version: 0, currency: null, signedOn: null, stages: [], total: "0.00", placementId: null };
const set: Schemas["JobRetainer"] = {
  version: 1, currency: "MYR", signedOn: "2026-11-01", total: "30000.00", placementId: null, stages: [
    { label: "Engagement", amount: "15000.00", dueKind: "after_signing", dueOn: "2026-11-01", dueDays: 0, invoiceStatus: "paid", invoiceNumber: "INV-7", invoicedOn: "2026-11-01", paidOn: "2026-11-05" },
    { label: "Shortlist", amount: "15000.00", dueKind: "after_start", dueOn: null, dueDays: 14, invoiceStatus: "not_invoiced", invoiceNumber: null, invoicedOn: null, paidOn: null },
  ],
};

describe("JobRetainer", () => {
  it("is not shown without the fee permission", () => {
    const get = vi.spyOn(api, "GET");
    renderWithClient(<JobRetainer jobId="jo_1" permissions={["jobs:read"]} />);
    expect(screen.queryByRole("region", { name: "Retainer" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("lists the stages with when they are due", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(set));
    renderWithClient(<JobRetainer jobId="jo_1" permissions={["placements.fees:read"]} />);
    expect(await screen.findByText("Engagement")).toBeTruthy();
    expect(screen.getByText("14 days after the start")).toBeTruthy();
    expect(screen.getByText("MYR 30,000.00")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Edit" }), "changing it needs placements.fees:update").toBeNull();
  });

  it("adds a retainer from the usual three stages", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(none));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(set));
    renderWithClient(<JobRetainer jobId="jo_1" permissions={["placements.fees:read", "placements.fees:update"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Add retainer" }));
    fireEvent.change(screen.getByLabelText("Stage 1 amount"), { target: { value: "15,000" } });
    fireEvent.change(screen.getByLabelText("Stage 1 due"), { target: { value: "after_signing" } });
    fireEvent.change(screen.getByLabelText("Stage 1 days"), { target: { value: "0" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findAllByText("Enter an amount like 15000 or 15,000.")).toHaveLength(2);
    expect(put).not.toHaveBeenCalled(); // the other two stages have names but no amounts
    fireEvent.click(screen.getByRole("button", { name: "Remove stage 3" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove stage 2" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ version: 0, currency: "MYR", signedOn: null, stages: [
      { label: "Engagement", amount: "15000", dueKind: "after_signing", dueDays: 0, invoiceStatus: "not_invoiced" },
    ] });
    expect(await screen.findByText("Shortlist")).toBeTruthy();
  });
});
