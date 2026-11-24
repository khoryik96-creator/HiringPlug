import { describe, expect, it } from "vitest";

import { changeLines, exportedList, recordHref, whatHappened } from "./auditText";
import { changesQuery, defaultFilters, readView, salaryQuery, viewHash } from "./filters";
import { dayText } from "./when";

const said = (action: string, changes: Record<string, unknown> = {}) => whatHappened({ action, changes });

describe("audit log sentences", () => {
  it("has its own words for the common entries", () => {
    expect(said("user.signed_in")).toBe("Signed in");
    expect(said("application.moved", { stage: { from: "applied", to: "interview" } })).toBe("Moved to Interview");
    expect(said("candidate.exported", { count: 212, salaries: true })).toBe("Exported candidates (212)");
    expect(said("company.exported", { count: 3 })).toBe("Exported companies (3)");
    expect(said("candidate.compensation_updated", { compensation: { changed: true, redacted: true } })).toBe("Changed the salary details");
    expect(said("email.queued", { salary: true })).toBe("Sent an email with salary");
  });

  it("builds a sentence from any other action, and never breaks", () => {
    expect(said("candidate.updated")).toBe("Changed a candidate");
    expect(said("job.created")).toBe("Added a job");
    expect(said("interview.scheduled")).toBe("Scheduled an interview");
    expect(said("email_template.created")).toBe("Added an email template");
    expect(said("user.invited")).toBe("Invited a team member");
    expect(said("candidate.industries_saved")).toBe("Saved a candidate's industries");
    expect(said("notification.settings_changed")).toBe("Changed the notification settings");
    expect(said("settings.salary_calculation_changed")).toBe("Changed the salary calculation settings");
    expect(said("settings.backup_downloaded")).toBe("Downloaded a backup");
    expect(said("invoice_settings.saved")).toBe("Saved the invoice settings");
    expect(said("ai_usage.warning_reached")).toBe("AI usage: warning reached");
    expect(said("odd")).toBe("Odd: odd");
  });

  it("shows what changed, from -> to, and a hidden salary only as changed", () => {
    expect(changeLines({
      noticePeriod: { from: { value: 1, unit: "month" }, to: { value: 2, unit: "month" } },
      compensation: { changed: true, redacted: true },
      subject: { changed: true, redacted: true },
      stage: { from: "applied", to: "interview" },
      salaries: true, count: 89,
    })).toEqual([
      { field: "Notice period", kind: "change", from: "1 month", to: "2 months" },
      { field: "Compensation", kind: "hidden", amount: true },
      { field: "Subject", kind: "hidden", amount: false },
      { field: "Stage", kind: "change", from: "Applied", to: "Interview" },
      { field: "Salaries", kind: "detail", value: "yes" },
      { field: "Count", kind: "detail", value: "89" },
    ]);
  });

  it("links candidates, jobs, companies and contacts to their pages", () => {
    expect(recordHref({ entityType: "candidate", entityId: "cand_01abc", candidateId: null })).toBe("#/candidates/cand_01abc");
    expect(recordHref({ entityType: "job_order", entityId: "jo_01abc", candidateId: null })).toBe("#/jobs/jo_01abc");
    expect(recordHref({ entityType: "email", entityId: "em_01abc", candidateId: null })).toBeNull();
    expect(recordHref({ entityType: "application", entityId: "appl_01abc", candidateId: "cand_01abc" }), "a candidate on a job").toBe("#/candidates/cand_01abc");
    expect(recordHref({ entityType: "application", entityId: "appl_01abc", candidateId: null })).toBeNull();
    expect(exportedList("candidate.exported")).toBe("Candidate list");
    expect(exportedList("candidate.updated")).toBeNull();
    expect(exportedList("auditlog.exported")).toBe("Audit log list");
  });
});

describe("audit log filters", () => {
  const now = new Date(2026, 10, 24, 15, 0);

  it("defaults to the last 30 days and keeps the view in the address", () => {
    expect(defaultFilters(now)).toEqual({ who: "", what: "", from: "2026-10-26", to: "2026-11-24" });
    const view = readView("#/settings/audit-log?tab=salary&who=usr_1&what=export&from=2026-11-01&to=", true, now);
    expect(view).toEqual({ tab: "salary", filters: { who: "usr_1", what: "export", from: "2026-11-01", to: "" } });
    expect(readView(viewHash(view.tab, view.filters), true, now)).toEqual(view);
    expect(readView("#/settings/audit-log?tab=salary&what=export", false, now).tab).toBe("changes");
    expect(readView("#/settings/audit-log?what=export&from=nonsense", true, now).filters).toEqual({ ...defaultFilters(now), what: "" });
  });

  it("turns What into the action the API finds", () => {
    const later = new Date(2026, 11, 2, 9, 0);
    const f = { who: "system", what: "sign-ins", from: "2026-11-01", to: "2026-11-24" };
    expect(changesQuery(f, later)).toEqual({ actor: "system", action: "user.signed_in", from: "2026-11-01", to: "2026-11-24" });
    expect(changesQuery({ ...f, who: "", what: "pipeline", from: "" }, later)).toEqual({ action: "application", to: "2026-11-24" });
    // Some kinds are written under several actions: formatted CVs, and each settings page's own.
    expect(changesQuery({ ...f, who: "", what: "documents" }, later).action?.split(",")).toEqual(expect.arrayContaining(["document", "cv"]));
    expect(changesQuery({ ...f, who: "", what: "settings" }, later).action?.split(",")).toEqual(
      expect.arrayContaining(["settings", "invoice_settings", "tenant.renamed"]));
    expect(salaryQuery({ ...f, who: "usr_1", what: "candidate_list" }, later)).toEqual({ user: "usr_1", kind: "candidate_list", from: "2026-11-01", to: "2026-11-24" });
  });

  it("sends no end when To is today (the API's days are the agency's, which may still be on)", () => {
    const today = new Date(2026, 10, 24, 23, 30);
    const f = { who: "", what: "", from: "2026-11-01", to: "2026-11-24" };
    expect(changesQuery(f, today)).toEqual({ from: "2026-11-01" });
    expect(salaryQuery(f, today)).toEqual({ from: "2026-11-01" });
    expect(changesQuery({ ...f, to: "2026-11-23" }, today)).toEqual({ from: "2026-11-01", to: "2026-11-23" });
  });

  it("writes days as today, yesterday, or the date", () => {
    expect(dayText("2026-11-24", now)).toBe("Today");
    expect(dayText("2026-11-23", now)).toBe("Yesterday");
    expect(dayText("2026-11-20", now)).toBe("Fri 20 Nov");
    expect(dayText("2025-12-31", now)).toBe("31 Dec 2025");
  });
});
