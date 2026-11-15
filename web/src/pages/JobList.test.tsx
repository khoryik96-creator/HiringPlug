import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { attention } from "../components/JobProgressTable";
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
    const p = (c: ReturnType<typeof counts>, lastMovedAt: string | null, waiting = 0) => ({ jobId: "j", stageCounts: c, lastMovedAt, waiting });
    expect(attention(job("j", "A", 21), p(counts({}), null))).toBe("No candidates in 3 weeks");
    expect(attention(job("j", "A", 5), p(counts({}), null))).toBeNull();
    expect(attention(job("j", "A", 30), p(counts({ submitted: 3 }), ago(9), 3))).toBe("3 waiting over 7 days");
    expect(attention(job("j", "A", 30), p(counts({ placed: 1 }), ago(10)))).toBe("Nothing moved in 10 days");
    expect(attention(job("j", "A", 30), p(counts({ applied: 2 }), ago(1)))).toBeNull();
    expect(attention({ ...job("j", "A", 30), status: "on_hold" }, p(counts({}), null))).toBeNull();
  });

  it("shows each job's pipeline and owner, and the ones needing attention on their own", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/jobs"
      ? { items: [job("jo_1", "Risk Analyst", 34), job("jo_2", "Senior Accountant", 5, "Mei Kee")], nextCursor: null }
      : { items: [
        { jobId: "jo_1", stageCounts: counts({ applied: 1, screening: 2, submitted: 3 }), lastMovedAt: ago(9), waiting: 3 },
        { jobId: "jo_2", stageCounts: counts({ applied: 7 }), lastMovedAt: ago(0), waiting: 0 },
      ] }))) as never);
    renderWithClient(<JobList canCreate={false} />);
    const risk = (await screen.findByRole("link", { name: "Risk Analyst" })).closest("tr") as HTMLElement;
    expect(await within(risk).findByText("1 applied · 2 screening · 3 submitted")).toBeTruthy();
    expect(risk.className).toContain("needs-attention");
    expect(risk.textContent).toContain("3 waiting over 7 days");
    expect(risk.textContent).toContain("34 days");
    expect(within(risk).getByLabelText("Demo Recruiter").textContent).toBe("DR");
    expect((get.mock.calls as unknown[][]).find((c) => c[0] === "/v1/pipeline/jobs")?.[1]).toEqual({ params: { query: { jobId: ["jo_1", "jo_2"] } } });

    fireEvent.click(screen.getByRole("button", { name: "Needs attention 1" }));
    expect(screen.queryByRole("link", { name: "Senior Accountant" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Mine" }));
    expect((get.mock.calls as unknown[][]).some((c) => c[0] === "/v1/jobs" && (c[1] as { params: { query: { mine?: boolean } } }).params.query.mine === true)).toBe(true);
  });
});
