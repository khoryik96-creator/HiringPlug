import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Application, type Permission } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { PipelineBoard } from "./PipelineBoard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const siti: Application = {
  id: "appl_01jbz6w0m0k8q9x2r3s4t5v6w1", version: 3, jobId: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", jobTitle: "Auditor", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7",
  companyName: "Maybank", candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w1", candidateName: "Siti Aisyah", candidateTitle: "Accountant", stage: "interview",
  reason: null, reasonNote: null, channel: null, stageChangedAt: new Date().toISOString(), createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};
const permissions: Permission[] = ["pipeline:read", "pipeline:update", "offers:manage", "offers.salary:update", "placements:manage"];

function board() {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/jobs/{jobId}/applications") return Promise.resolve(ok({ items: [siti] }));
    if (path === "/v1/applications/{applicationId}/placement-defaults") {
      return Promise.resolve(ok({ companyId: siti.companyId, companyName: "Maybank", feePercent: null, paymentTermsDays: null, guaranteeDays: null, contactId: null, termsSet: false, tiers: [], salaryCurrency: "MYR" }));
    }
    return Promise.resolve(ok({ items: [] }));
  }) as never);
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
  renderWithClient(<PipelineBoard jobId={siti.jobId} canUpdate permissions={permissions} />);
  return post;
}
const moveTo = async (stage: string) => {
  fireEvent.click((await screen.findByRole("link", { name: "Siti Aisyah" })).closest(".card") as Element);
  fireEvent.change(screen.getByRole("combobox", { name: "Move the chosen candidates" }), { target: { value: stage } });
};
const paths = (post: ReturnType<typeof board>) => post.mock.calls.map((c) => (c as unknown[])[0]);

describe("moving to offer or placed asks for the details first", () => {
  it("asks for the offered salary; Cancel leaves the candidate where they were, saving drafts the offer", async () => {
    const post = board();
    await moveTo("offer");
    const prompt = await screen.findByRole("region", { name: "Offer for Siti Aisyah" });
    fireEvent.click(within(prompt).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("region", { name: "Offer for Siti Aisyah" })).toBeNull();
    expect(post).not.toHaveBeenCalled(); // not moved

    await moveTo("offer");
    const again = await screen.findByRole("region", { name: "Offer for Siti Aisyah" });
    fireEvent.change(within(again).getByLabelText("Salary"), { target: { value: "12,000" } });
    fireEvent.click(within(again).getByRole("button", { name: "Draft offer" }));
    await waitFor(() => { expect(paths(post)).toEqual(["/v1/applications/{applicationId}/offers"]); });
    expect(sentBody(post)).toMatchObject({ salary: { amount: "12000", currency: "MYR", period: "month" } });
  });

  it("asks for the placement report; without it the candidate goes back to Offer", async () => {
    const post = board();
    await moveTo("placed");
    const prompt = await screen.findByRole("region", { name: "Placement report for Siti Aisyah" });
    fireEvent.click(within(prompt).getByRole("button", { name: "Cancel" }));
    await waitFor(() => { expect(paths(post)).toEqual(["/v1/applications/{applicationId}/move"]); });
    expect(sentBody(post)).toEqual({ version: 3, stage: "offer" });
  });
});
