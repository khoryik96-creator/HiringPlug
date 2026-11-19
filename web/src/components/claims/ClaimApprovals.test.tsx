import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import { ok, renderWithClient, sentBody, testClaim, testFieldSettings } from "../../test-utils";
import { ClaimApprovals } from "./ClaimApprovals";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function serve(items: Schemas["Claim"][]) {
  return vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/claims/team" ? ok({ items, total: items.length, totals: [{ currency: "MYR", amount: "45.00" }] })
      : path === "/v1/claims/team/summary" ? ok({ facts: [], people: [{ id: "usr_1", name: "Aisyah Rahman" }], records: [] })
        : testFieldSettings(),
  ));
}

const waiting = testClaim({ id: "clm_wait", status: "submitted", merchant: "Grab", claimedAt: "2026-11-04T01:00:00Z" });
const approved = testClaim({ id: "clm_ok", status: "approved", merchant: "Nasi Kandar", amount: "22.00", version: 5 });

it("approves what is waiting and pays what is approved", async () => {
  serve([waiting, approved]);
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
  renderWithClient(<ClaimApprovals />);
  fireEvent.click(await screen.findByRole("checkbox", { name: "Select all" }));
  expect(screen.getByRole("group", { name: "Selected claims" }).textContent).toContain("2 selected · MYR 45.00");
  fireEvent.click(screen.getByRole("button", { name: "Approve 1" }));
  await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
  expect(sentBody(post)).toEqual({ claims: [{ id: "clm_wait", version: 3 }], decision: "approve" });

  fireEvent.click(screen.getByRole("checkbox", { name: "Select Aisyah Rahman's Nasi Kandar" }));
  fireEvent.click(screen.getByRole("button", { name: "Mark 1 paid" }));
  await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
  expect(sentBody(post, 1)).toEqual({ claims: [{ id: "clm_ok", version: 5 }], decision: "pay" });
});

it("rejects only with a reason", async () => {
  serve([waiting]);
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
  renderWithClient(<ClaimApprovals />);
  fireEvent.click(await screen.findByRole("checkbox", { name: "Select Aisyah Rahman's Grab" }));
  fireEvent.click(screen.getByRole("button", { name: "Reject…" }));
  const reject = screen.getByRole("button", { name: "Reject 1" });
  expect((reject as HTMLButtonElement).disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Why they are rejected"), { target: { value: "Attach the itemised receipt" } });
  fireEvent.click(reject);
  await waitFor(() => { expect(post).toHaveBeenCalled(); });
  expect(sentBody(post)).toEqual({ claims: [{ id: "clm_wait", version: 3 }], decision: "reject", note: "Attach the itemised receipt" });
});
