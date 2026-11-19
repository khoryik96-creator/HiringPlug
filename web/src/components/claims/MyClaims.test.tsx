import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import * as files from "../../api/files";
import { ok, RECRUITER, renderWithClient, sentBody, testClaim, testFieldSettings } from "../../test-utils";
import { MyClaims } from "./MyClaims";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function serve(items: Schemas["Claim"][]) {
  return vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/claims" ? ok({ items, total: items.length, totals: [] }) : testFieldSettings(),
  ));
}

it("shows what was read from a receipt as a suggestion, and saves a change with the claim's version", async () => {
  serve([testClaim()]);
  const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testClaim({ merchant: "Grab ride to Acme", version: 4, fromReceipt: ["amount", "receiptType"] })));
  renderWithClient(<MyClaims permissions={RECRUITER} />);
  const row = await screen.findByRole("article", { name: "Claim grab.png" });
  const amount = within(row).getByLabelText("Amount");
  expect(amount.className).toContain("from-receipt");
  expect(within(row).getByLabelText("Purpose").className).not.toContain("from-receipt");
  const merchant = within(row).getByLabelText("Merchant");
  fireEvent.change(merchant, { target: { value: "Grab ride to Acme" } });
  fireEvent.blur(merchant);
  await waitFor(() => { expect(patch).toHaveBeenCalled(); });
  expect(sentBody(patch)).toEqual({ merchant: "Grab ride to Acme", version: 3 });
});

it("submits only complete claims, all chosen at once", async () => {
  serve([
    testClaim(),
    testClaim({ id: "clm_reading", fileName: "lunch.jpg", ocrStatus: "reading", amount: null, missing: ["amount", "purpose"] }),
    testClaim({ id: "clm_open", fileName: "parking.pdf", purpose: null, missing: ["purpose"] }),
    testClaim({ id: "clm_sent", fileName: "taxi.png", status: "submitted", claimedAt: "2026-11-04T02:00:00Z" }),
  ]);
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
  renderWithClient(<MyClaims permissions={RECRUITER} />);
  const reading = await screen.findByRole("article", { name: "Claim lunch.jpg" });
  expect(within(reading).getByRole("status").textContent).toBe("Reading the receipt…");
  expect(within(reading).getByRole<HTMLInputElement>("checkbox").disabled).toBe(true);
  const open = screen.getByRole("article", { name: "Claim parking.pdf" });
  expect(within(open).getByText("To submit, fill in the purpose.")).toBeTruthy();
  expect(within(open).getByLabelText<HTMLInputElement>("Purpose").dataset.missing).toBe("");

  fireEvent.click(screen.getByRole("button", { name: "Select all ready (1)" }));
  fireEvent.click(screen.getByRole("button", { name: /^Submit 1 claim · MYR 23\.00/ }));
  await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/claims/submit", expect.anything()); });
  expect(sentBody(post)).toEqual({ claims: [{ id: "clm_ready", version: 3 }] });
  expect(screen.getByRole("table").textContent).toContain("Waiting for approval");
});

it("uploads each dropped receipt", async () => {
  serve([]);
  const upload = vi.spyOn(files, "uploadClaimReceipt").mockResolvedValue(testClaim());
  renderWithClient(<MyClaims permissions={RECRUITER} />);
  const input = await screen.findByLabelText("Drop receipts here");
  const receipts = [new File(["a"], "grab.png", { type: "image/png" }), new File(["b"], "lunch.pdf", { type: "application/pdf" }),
    new File(["c"], "notes.docx")];
  fireEvent.change(input, { target: { files: receipts } });
  await waitFor(() => { expect(upload).toHaveBeenCalledTimes(2); });
  expect(upload.mock.calls.map((c) => c[1])).toEqual(["grab.png", "lunch.pdf"]);
  expect(await screen.findByText(/Skipped/)).toBeTruthy();
});
