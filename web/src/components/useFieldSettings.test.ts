import { describe, expect, it } from "vitest";

import { testFieldSettings } from "../test-utils";
import { fieldHelpers, labelFor, noticeKey } from "./useFieldSettings";

describe("field settings helpers", () => {
  it("offers visible options, keeps a hidden or unlisted current value, and falls back to built-ins", () => {
    const data = testFieldSettings([{ form: "candidate", field: "email" }]) as unknown as Parameters<typeof fieldHelpers>[0];
    data?.lists[0]?.options.push({ key: "job_fair", label: "Job fair", hint: null, hidden: true, builtIn: false });
    const fs = fieldHelpers(data);
    expect(fs.options("candidate_source").map((o) => o.key)).not.toContain("job_fair");
    expect(fs.options("candidate_source", "job_fair").at(-1)).toMatchObject({ key: "job_fair", label: "Job fair" });
    expect(fs.options("candidate_source", "roadshow").at(-1)).toEqual({ key: "roadshow", label: "roadshow" });
    expect(fs.label("candidate_source", "job_fair")).toBe("Job fair");
    expect(fs.mandatory("candidate", "email")).toBe(true);
    expect(fs.mandatory("candidate", "phone")).toBe(false);

    const loading = fieldHelpers(undefined);
    expect(loading.label("work_type", "full_time")).toBe("Full time");
    expect(loading.options("candidate_source").map((o) => o.key)).toContain("careers_page");
    expect(loading.options("notice_period").map((o) => o.key)).toContain("1_month");
    expect(loading.options("industry")).toHaveLength(10);
    expect(fs.options("notice_period", "5_week").at(-1)).toEqual({ key: "5_week", label: "5 weeks" });
    expect(fs.label("candidate_source", "indeed")).toBe("Indeed"); // an application channel, in reports
    expect(labelFor("Email", true)).toBe("Email *");
    expect(noticeKey("2", "week")).toBe("2_week");
  });
});
