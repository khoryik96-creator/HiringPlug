import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, RECRUITER, renderWithClient, sentBody, testMe } from "../test-utils";
import { ApplicationOffers, OfferItem } from "./Offers";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const me = testMe(RECRUITER); // usr_01jbz6w0m0k8q9x2r3s4t5v6w7
const offer = (over: Partial<Schemas["Offer"]> = {}): Schemas["Offer"] => ({
  id: "ofr_1", version: 2, applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah", jobId: "jo_1",
  jobTitle: "Accountant", companyName: "Contoh Berhad", status: "draft", salary: { amount: "9000.00", currency: "MYR", period: "month" },
  startDate: "2026-12-01", expiresOn: "2026-11-15", terms: "Medical", requestedBy: null, requestedByName: null,
  decidedBy: null, decidedByName: null, decidedAt: null, decisionNote: null, sentAt: null, answeredAt: null,
  restrictedFields: [], createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  ...over,
});
const reads = (items: Schemas["Offer"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/me" ? me : { items }))) as never);

const manage = ["offers:read", "offers:manage", "offers.salary:read", "offers.salary:update"] as const;

describe("offers", () => {
  it("drafts an offer with its salary", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(offer()));
    renderWithClient(<ApplicationOffers applicationId="appl_1" stage="interview" permissions={[...manage]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Make offer" }));
    fireEvent.change(screen.getByLabelText("Salary"), { target: { value: "9,000" } });
    fireEvent.change(screen.getByLabelText("Start date"), { target: { value: "2026-12-01" } });
    fireEvent.click(screen.getByRole("button", { name: "Draft offer" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ startDate: "2026-12-01", salary: { amount: "9000", currency: "MYR", period: "month" } });
  });

  it("recruiters ask for approval; they cannot send a draft", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(offer({ status: "pending_approval" })));
    renderWithClient(<OfferItem offer={offer()} permissions={[...manage]} />);
    expect(screen.getByText(/MYR 9,000.00 a month/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Mark as sent" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Ask for approval" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 2, step: "request_approval" });
  });

  it("an approver approves someone else's request, with a note", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(offer({ status: "approved" })));
    renderWithClient(<OfferItem offer={offer({ status: "pending_approval", requestedBy: "usr_rec", requestedByName: "Aina" })} permissions={["offers:read", "offers:approve", "offers.salary:read"]} />);
    expect(screen.getByText("Approval asked by Aina.")).toBeTruthy();
    fireEvent.change(await screen.findByLabelText("Note"), { target: { value: "Within band" } });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 2, approve: true, note: "Within band" });
    expect(screen.queryByRole("button", { name: "Change" })).toBeNull(); // approvers who cannot manage only decide
  });

  it("does not offer a decision on your own request", async () => {
    reads([]);
    renderWithClient(<OfferItem offer={offer({ status: "pending_approval", requestedBy: me.user.id })} permissions={["offers:read", "offers:approve"]} />);
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Approve" })).toBeNull(); });
  });
});
