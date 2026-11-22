import { describe as group, expect, it } from "vitest";

import { actorOf, describe } from "./activityText";

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

group("client shortlist links in the timeline", () => {
  it("says what the recruiter and the client did", () => {
    expect(describe(entry("client_shortlist.created", { candidates: 2, emailedTo: "aisyah@maju.test" })))
      .toBe("made a client link for 2 candidates and emailed it to aisyah@maju.test");
    expect(describe(entry("client_shortlist.shared", { jobTitle: "Finance Manager", blind: true }))).toBe("put them on a client link for Finance Manager (blind CV)");
    expect(describe(entry("client_shortlist.answered", { decision: "approved", candidateName: "Siti Tan", comment: "Let's meet." })))
      .toBe("approved Siti Tan on the shortlist link: “Let's meet.”");
    expect(describe(entry("client_shortlist.answered", { decision: "rejected", jobTitle: "Finance Manager", comment: null })))
      .toBe("rejected them for Finance Manager on the shortlist link");
    expect(describe(entry("client_shortlist.revoked", {}))).toBe("revoked a client link");
  });

  it("names the client as who opened and answered", () => {
    expect(actorOf({ ...entry("client_shortlist.opened", {}), actorName: "System" })).toBe("The client");
    expect(actorOf({ ...entry("client_shortlist.created", {}), actorName: "Aina" })).toBe("Aina");
  });
});
