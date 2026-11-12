import { describe, expect, it } from "vitest";

import type { Schemas } from "../api/client";
import { automationBody, automationForm, blankAction, roughFill, thenText, timingText, toggled, whenText } from "./automationText";

const au = (over: Partial<Schemas["Automation"]> = {}): Schemas["Automation"] => ({
  id: "aut_1", version: 3, name: "Not progressing", enabled: false, trigger: "stage_changed", stage: "rejected", offsetDays: 2,
  companyId: null, companyName: null, sendMode: "approve", preset: "not_progressing", createdAt: "", updatedAt: "",
  actions: [{ kind: "email_candidate", subject: "S", body: "B" }], ...over,
});

describe("automations in words", () => {
  it("says when, how long and what", () => {
    expect(whenText(au())).toBe("A candidate moves to Rejected");
    expect(timingText(au())).toBe("2 days after");
    expect(timingText(au({ trigger: "interview_scheduled", offsetDays: -1 }))).toBe("1 day before");
    expect(timingText(au({ trigger: "application_received", offsetDays: 0 }))).toBe("straight away");
    expect(timingText(au({ trigger: "placement_starts", offsetDays: 0 }))).toBe("on the day");
    expect(thenText([{ kind: "email_candidate" }, { kind: "task" }])).toBe("email the candidate and give the recruiter a task");
  });

  it("edits an automation and sends what the form shows", () => {
    const f = automationForm(au({ trigger: "guarantee_ends", stage: null, offsetDays: -7 }), "send");
    expect(f).toMatchObject({ days: "7", direction: "before", sendMode: "approve" });
    expect(automationBody({ ...f, name: " Guarantee " }, 3)).toEqual({
      version: 3, name: "Guarantee", enabled: false, trigger: "guarantee_ends", offsetDays: -7, companyId: null,
      actions: [{ kind: "email_candidate", subject: "S", body: "B" }], sendMode: "approve",
    });
    const fresh = automationForm(undefined, "send");
    expect(fresh.sendMode, "new ones start from the default").toBe("send");
    const body = automationBody({ ...fresh, name: "Invite", days: "3", direction: "before", actions: [{ ...blankAction("task"), title: " Call " }] });
    expect(body).toMatchObject({ trigger: "stage_changed", stage: "interview", offsetDays: 3, actions: [{ kind: "task", title: "Call" }] });
    expect(body, "no version for a new one").not.toHaveProperty("version");
  });

  it("switches one on, keeping the rest", () => {
    expect(toggled(au(), true)).toMatchObject({ version: 3, enabled: true, stage: "rejected", offsetDays: 2, sendMode: "approve" });
  });

  it("fills what the list knows of a subject", () => {
    expect(roughFill("Thank you, {{candidate.firstName}}: {{job.title}} at {{company.name}} {{interview.when}}",
      { candidateName: "Siti Aisyah", jobTitle: "Accountant", companyName: "Maju Foods" })).toBe("Thank you, Siti: Accountant at Maju Foods {{interview.when}}");
  });
});
