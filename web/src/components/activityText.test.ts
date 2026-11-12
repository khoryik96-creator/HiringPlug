import { describe as group, expect, it } from "vitest";

import { describe } from "./activityText";

const entry = (action: string, changes: Record<string, unknown>) =>
  (({ id: "aud_1", action, at: "2026-11-12T04:00:00Z", actorName: "Aina", changes }));

group("logged screening calls in the timeline", () => {
  it("reads as a sentence, not the raw record", () => {
    expect(describe(entry("candidate.screened", { calledBy: "usr_01m76w5z", calledOn: "2026-11-12", note: true, salary: true, salarySentButOlder: false })))
      .toBe("logged a screening call on 12 Nov 2026 and updated the salary");
    expect(describe(entry("candidate.screened", { calledOn: "2026-11-05", note: true, salary: false, salarySentButOlder: true })))
      .toBe("logged a screening call on 5 Nov 2026 (its salary was kept with the call; a later call's salary stays current)");
    expect(describe(entry("candidate.screened", { calledOn: "2026-11-05", note: true, salary: false, salarySentButOlder: false })))
      .toBe("logged a screening call on 5 Nov 2026");
  });
});
