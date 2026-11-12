import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { MyTasks } from "../components/MyTasks";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Automations } from "./Automations";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const au: Schemas["Automation"] = {
  id: "aut_1", version: 2, name: "Not progressing, 2 days after", enabled: false, trigger: "stage_changed", stage: "rejected", offsetDays: 2,
  companyId: null, companyName: null, sendMode: "approve", preset: "not_progressing", createdAt: "", updatedAt: "",
  actions: [{ kind: "email_candidate", subject: "Your application", body: "Dear {{candidate.firstName}}" }],
};
const waiting: Schemas["AutomationRun"] = {
  id: "arn_1", automationId: "aut_1", automationName: "Application received", status: "to_approve", dueAt: "2026-11-12T03:00:00Z",
  applicationId: "appl_1", candidateId: "cand_1", candidateName: "Siti Aisyah", jobTitle: "Accountant", companyName: "Maju Foods",
  actions: [{ kind: "email_candidate", subject: "Thank you for applying", body: "B" }], detail: null, finishedAt: null,
};
const reads = (runs: Schemas["AutomationRun"][]) => vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
  path === "/v1/automations" ? { items: [au], settings: { defaultSendMode: "approve" } }
    : path === "/v1/automation-runs" ? { items: runs }
      : path === "/v1/tasks" ? { items: [{ id: "tsk_1", title: "Call Siti", dueOn: "2026-11-12", doneAt: null, applicationId: "appl_1",
        candidateId: "cand_1", candidateName: "Siti Aisyah", jobTitle: "Accountant", automationName: "Check-in", createdAt: "" }] }
        : { items: [] },
))) as never);

describe("automations", () => {
  it("reads as When · If · Then, and switches on", async () => {
    reads([]);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...au, enabled: true }));
    renderWithClient(<Automations permissions={["automations:read", "automations:manage"]} />);
    expect(await screen.findByText("Not progressing, 2 days after")).toBeTruthy();
    expect(screen.getByText(/a candidate moves to rejected/)).toBeTruthy();
    expect(screen.getByText("Asks first")).toBeTruthy();
    fireEvent.click(screen.getByRole("switch", { name: "Not progressing, 2 days after on" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ version: 2, enabled: true, stage: "rejected", sendMode: "approve" });
  });

  it("sends an email waiting for approval", async () => {
    reads([waiting]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...waiting, status: "done" }));
    renderWithClient(<Automations permissions={["automations:read", "automations:approve"]} />);
    expect(await screen.findByText("Siti Aisyah")).toBeTruthy();
    expect(screen.getByText(/“Thank you for applying”/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "+ New automation" }), "changing them needs automations:manage").toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/automation-runs/{runId}/send", { params: { path: { runId: "arn_1" } } }); });
  });

  it("lists your tasks and ticks them off", async () => {
    reads([waiting]);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({}));
    renderWithClient(<MyTasks approvals />);
    expect(await screen.findByText(/Call Siti/)).toBeTruthy();
    expect(await screen.findByText("1 email to approve")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox"));
    await waitFor(() => { expect(sentBody(patch)).toEqual({ done: true }); });
  });
});
