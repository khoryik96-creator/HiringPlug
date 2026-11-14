import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Privacy } from "./Privacy";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const request = (over: Partial<Schemas["PrivacyRequest"]>): Schemas["PrivacyRequest"] => ({
  id: "prq_1", version: 1, candidateId: "cand_1", candidateName: "Siti", kind: "access", receivedOn: "2026-10-01", details: null,
  status: "open", outcome: null, dueOn: "2026-10-22", overdue: true, daysOpen: 34, closedAt: null, closedByName: null, createdAt: "2026-10-01T00:00:00Z", ...over,
});
const overview: Schemas["PrivacyOverview"] = {
  consent: { given: 12, withdrawn: 1, notRecorded: 40 },
  openRequests: [request({}), request({ id: "prq_2", candidateId: "cand_2", candidateName: "Wei", kind: "withdraw_consent", dueOn: null, overdue: false, daysOpen: 0 })],
  closedRequests: [request({ id: "prq_3", status: "done", outcome: "Sent a copy.", closedAt: "2026-10-05T03:00:00Z", closedByName: "Owner" })],
  withdrawn: [{ candidateId: "cand_9", candidateName: "Ali", day: "2026-09-01" }],
};

describe("data protection dashboard", () => {
  it("shows the counts, what is overdue, and who asked not to be contacted", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(overview));
    renderWithClient(<Privacy />);
    expect((await screen.findByText("no consent recorded")).previousSibling?.textContent).toBe("40");
    const access = screen.getByRole("article", { name: "A copy of their data: Siti" });
    expect(access.textContent).toContain("overdue since 22 Oct 2026");
    expect(screen.getByRole("article", { name: "Stop using their data and contacting them: Wei" }).textContent).toContain("Done also marks them do not contact");
    expect(within(screen.getByRole("region", { name: "Do not contact" })).getByText("Ali")).toBeTruthy();
    expect(within(screen.getByRole("region", { name: "Recently closed" })).getByText("Sent a copy.")).toBeTruthy();
  });

  it("closes a request only with what was done, and can reopen one", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(overview));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(request({ status: "done" })));
    renderWithClient(<Privacy />);
    const access = await screen.findByRole("article", { name: "A copy of their data: Siti" });
    const done = within(access).getByRole("button", { name: "Done" });
    fireEvent.click(done);
    fireEvent.click(within(access).getByRole("button", { name: "Decline" }));
    expect(patch).not.toHaveBeenCalled();
    expect(within(access).getByLabelText<HTMLInputElement>("What was done").validity.valueMissing).toBe(true);
    fireEvent.change(within(access).getByLabelText("What was done"), { target: { value: "Emailed a copy." } });
    fireEvent.click(done);
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 1, status: "done", outcome: "Emailed a copy." });

    fireEvent.click(within(screen.getByRole("region", { name: "Recently closed" })).getByRole("button", { name: "Reopen" }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
    expect(sentBody(patch, 1)).toEqual({ version: 1, status: "open" });
  });
});
