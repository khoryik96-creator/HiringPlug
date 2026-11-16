import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { attention, goodNews } from "../components/JobProgressTable";
import { ok, renderWithClient } from "../test-utils";
import { JobList } from "./JobList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const ago = (d: number) => new Date(Date.now() - d * 86_400_000).toISOString();
const job = (id: string, title: string, openDays: number, ownerName: string | null = "Demo Recruiter") => ({
  id, title, companyId: "comp_1", companyName: "Contoh Capital", status: "open" as const, location: "Kuala Lumpur", openings: 1,
  ownerId: ownerName ? "usr_1" : null, ownerName, createdAt: ago(openDays), updatedAt: ago(1),
});
const counts = (c: Partial<Record<"applied" | "screening" | "submitted" | "interview" | "offer" | "placed", number>>) =>
  ({ applied: 0, screening: 0, submitted: 0, interview: 0, offer: 0, placed: 0, ...c });

describe("the jobs list", () => {
  it("says why an open job needs attention", () => {
    const p = (c: ReturnType<typeof counts>, lastMovedAt: string | null, waiting = 0) => ({ jobId: "j", stageCounts: c, lastMovedAt, waiting, waitingCounts: counts({}) });
    expect(attention(job("j", "A", 21), p(counts({}), null))).toBe("No candidates in 3 weeks");
    expect(attention(job("j", "A", 5), p(counts({}), null))).toBeNull();
    expect(attention(job("j", "A", 30), p(counts({ submitted: 3 }), ago(9), 3))).toBe("3 waiting over 7 days");
    expect(attention(job("j", "A", 30), p(counts({ placed: 1 }), ago(10)))).toBe("Nothing moved in 10 days");
    expect(attention(job("j", "A", 30), p(counts({ applied: 2 }), ago(1)))).toBeNull();
    expect(goodNews(p(counts({ submitted: 2, offer: 1 }), null))).toBe("1 offer out");
    expect(goodNews(p(counts({ submitted: 2 }), null))).toBe("2 with the client");
    expect(goodNews(p(counts({ applied: 2 }), null))).toBeNull();
    expect(attention({ ...job("j", "A", 30), status: "on_hold" }, p(counts({}), null))).toBeNull();
  });

  it("shows each job's candidates per stage as a heat map, the numbers on top, and the ones needing attention on their own", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/jobs"
      ? { items: [job("jo_1", "Risk Analyst", 34), job("jo_2", "Senior Accountant", 5, "Mei Kee")], nextCursor: null }
      : { items: [
        { jobId: "jo_1", stageCounts: counts({ applied: 1, screening: 2, submitted: 3 }), lastMovedAt: ago(9.5), waiting: 3, waitingCounts: counts({ submitted: 3 }) },
        { jobId: "jo_2", stageCounts: counts({ applied: 7 }), lastMovedAt: ago(0), waiting: 0, waitingCounts: counts({}) },
      ] }))) as never);
    renderWithClient(<JobList canCreate={false} />);
    const risk = (await screen.findByRole("link", { name: "Risk Analyst" })).closest("tr") as HTMLElement;
    const submitted = await within(risk).findByRole("link", { name: "3 in Submitted for Risk Analyst" });
    expect(submitted.getAttribute("href"), "a number opens the job at that stage").toBe("#/jobs/jo_1?stage=submitted");
    expect(submitted.className, "an amber ring: someone waiting over 7 days").toContain("stall");
    expect(within(risk).getByRole("link", { name: "2 in Screening for Risk Analyst" }).className).toContain("h2");
    expect(risk.className).toContain("needs-attention");
    expect(within(risk).getByText("3 waiting over 7 days").className).toContain("amber");
    expect(risk.textContent).toContain("34 days");
    expect(risk.textContent).toContain("9 days ago");
    const accountant = screen.getByRole("link", { name: "Senior Accountant" }).closest("tr") as HTMLElement;
    expect(within(accountant).getByRole("link", { name: "7 in Applied for Senior Accountant" }).className).toContain("h3");
    expect(accountant.textContent).toContain("today");

    const numbers = screen.getByRole("group", { name: "Job numbers" });
    const kpi = (label: string) => within(numbers).getByText(label).closest(".kpi")?.querySelector("strong")?.textContent;
    expect(kpi("Open jobs")).toBe("2");
    expect(kpi("Candidates in play")).toBe("13");
    expect(kpi("Waiting over 7 days")).toBe("3");
    expect(kpi("Average time open")).toBe("20 days");
    expect(within(numbers).getByText(/1 new this week/)).toBeTruthy();
    expect(screen.getByRole("row", { name: /^All shown/ }).textContent).toBe("All shown823000");
    expect(within(risk).getByLabelText("Demo Recruiter").textContent).toBe("DR");
    expect((get.mock.calls as unknown[][]).find((c) => c[0] === "/v1/pipeline/jobs")?.[1]).toEqual({ params: { query: { jobId: ["jo_1", "jo_2"] } } });

    fireEvent.click(screen.getByRole("button", { name: "Needs attention 1" }));
    expect(screen.queryByRole("link", { name: "Senior Accountant" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Mine" }));
    expect((get.mock.calls as unknown[][]).some((c) => c[0] === "/v1/jobs" && (c[1] as { params: { query: { mine?: boolean } } }).params.query.mine === true)).toBe(true);
  });
});
