import { describe, expect, it } from "vitest";

import type { Schemas } from "../api/client";
import { formatDay } from "../labels";
import { blankStage, dueText, retainerBody, retainerForm, retainerProblems } from "./retainerFormState";

describe("retainer form", () => {
  it("starts with the usual three stages", () => {
    expect(retainerForm(undefined).stages.map((s) => s.label)).toEqual(["Engagement", "Shortlist", "Completion"]);
  });

  it("sends the stages filled in, each with what its due rule and invoice state use", () => {
    const f = retainerForm(undefined);
    f.currency = "myr";
    f.signedOn = "2026-11-01";
    f.stages = [
      { ...blankStage("Engagement"), amount: "15,000", dueKind: "after_signing", dueDays: "0", dueOn: "2026-01-01", invoiceStatus: "paid", paidOn: "2026-11-10" },
      { ...blankStage("Shortlist"), amount: "15000", dueOn: "2026-12-15", invoiceNumber: "ignored" },
      blankStage(),
    ];
    const body = retainerBody(f, 2);
    expect(body).toEqual({
      version: 2, currency: "MYR", signedOn: "2026-11-01", stages: [
        { label: "Engagement", amount: "15000", dueKind: "after_signing", dueDays: 0, invoiceStatus: "paid", invoiceNumber: undefined, invoicedOn: undefined, paidOn: "2026-11-10" },
        { label: "Shortlist", amount: "15000", dueKind: "on_date", dueOn: "2026-12-15", invoiceStatus: "not_invoiced" },
      ],
    });
    expect(retainerProblems(body)).toEqual({});
    expect(retainerBody({ ...f, stages: [blankStage()] }, 2).stages, "all empty: no retainer").toEqual([]);
  });

  it("says what is missing", () => {
    const body = retainerBody({ currency: "RM", signedOn: "", stages: [{ ...blankStage(), amount: "abc" }, { ...blankStage("B"), amount: "1", dueKind: "after_start" }] }, 0);
    expect(Object.keys(retainerProblems(body)).sort()).toEqual(["currency", "stages.0.amount", "stages.0.dueOn", "stages.0.label", "stages.1.dueDays"]);
  });

  it("says when a stage is due", () => {
    const st = (over: Partial<Schemas["RetainerStage"]>) => ({ dueOn: null, dueDays: null, ...over }) as Schemas["RetainerStage"];
    expect(dueText(st({ dueKind: "on_date", dueOn: "2026-12-15" }), formatDay)).toBe("15 Dec 2026");
    expect(dueText(st({ dueKind: "after_start", dueDays: 14 }), formatDay)).toBe("14 days after the start");
    expect(dueText(st({ dueKind: "after_signing", dueDays: 0, dueOn: "2026-11-01" }), formatDay)).toBe("1 Nov 2026 · on signing");
  });
});
