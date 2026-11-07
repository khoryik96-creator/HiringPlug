import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type JobOrder } from "../api/client";
import { JobDetail } from "../pages/JobDetail";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { JobForm } from "./JobForm";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const job: JobOrder = {
  id: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 2, companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank",
  contactId: null, contactName: null, title: "Senior Auditor", status: "open", location: "Kuala Lumpur",
  workType: "full_time", workArrangement: "hybrid", openings: 2, description: "Audit banks.", country: "MY", intakeDate: "2026-11-01",
  salary: { version: 1, range: { min: "9000.00", max: "12000.00", currency: "MYR", period: "month" }, updatedAt: "2026-11-01T00:00:00Z" },
  restrictedFields: [], ownerId: null, ownerName: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

const change = (label: string, value: string) => {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
};

describe("job form", () => {
  it("adds a job at the company it was opened from", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok({ items: [], nextCursor: null }))) as never);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(job));
    const onSaved = vi.fn();
    renderWithClient(<JobForm job={null} company={{ id: job.companyId, name: "Maybank" }} onDone={vi.fn()} onSaved={onSaved} />);
    change("Job title", "Senior Auditor");
    change("Openings", "2");
    change("Work type", "full_time");
    fireEvent.click(screen.getByRole("button", { name: "Add job" }));
    await waitFor(() => { expect(onSaved).toHaveBeenCalledWith(job); });
    expect(sentBody(post)).toMatchObject({ companyId: job.companyId, title: "Senior Auditor", openings: 2, workType: "full_time", status: "open", contactId: null });
  });

  it("is filled in from a job description dropped on it, keeping what was typed", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok({ items: [], nextCursor: null }))) as never);
    const details = { title: "Finance Manager", location: "Petaling Jaya, Selangor", country: "MY", workType: "full_time", workArrangement: "hybrid",
      salaryText: "RM 12,000", closesOn: null, description: "Lead month-end closing." };
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: "jdf_1", jobId: null, fileName: "JD.txt", status: "read", error: null, details, createdAt: "2026-11-07T00:00:00Z",
    }), { status: 201 }));
    renderWithClient(<JobForm job={null} onDone={vi.fn()} />);
    change("Location", "Kuala Lumpur");
    const drop = screen.getByLabelText(/Drop the client's JD/);
    fireEvent.change(drop, { target: { files: [new File(["Position: Finance Manager"], "JD.txt", { type: "text/plain" })] } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Job title").value).toBe("Finance Manager"); });
    expect(fetch.mock.calls[0]?.[0]).toBe("/v1/jd-files");
    expect(screen.getByLabelText<HTMLInputElement>("Location").value).toBe("Kuala Lumpur");
    expect(screen.getByLabelText<HTMLSelectElement>("Where").value).toBe("hybrid");
    expect(screen.getByLabelText<HTMLTextAreaElement>("Description").value).toBe("Lead month-end closing.");
  });

  it("refuses a bad number of openings before sending", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(job));
    renderWithClient(<JobForm job={null} company={{ id: job.companyId, name: "Maybank" }} onDone={vi.fn()} />);
    change("Job title", "Auditor");
    change("Openings", "0");
    fireEvent.click(screen.getByRole("button", { name: "Add job" }));
    expect(await screen.findByText("Enter how many people to hire (1–999).")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });

  it("saves an edit with the version it was opened with", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok({ items: [], nextCursor: null }))) as never);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...job, version: 4 }));
    const onDone = vi.fn();
    const { rerender } = renderWithClient(<JobForm job={job} onDone={onDone} />);
    rerender(<JobForm job={{ ...job, version: 3 }} onDone={onDone} />);
    change("Status", "on_hold");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ version: 2, status: "on_hold" });
  });
});

describe("job page", () => {
  it("shows the budget to those who may see it", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok(job))) as never);
    renderWithClient(<JobDetail id={job.id} permissions={["jobs:read", "jobs:update", "jobs.salary:read", "jobs.salary:update"]} />);
    await screen.findByRole("heading", { name: /Senior Auditor/ });
    expect(screen.getByText("MYR 9000.00 – 12000.00 / month")).toBeTruthy();
    expect(screen.getByText("Kuala Lumpur, Malaysia (MY) · Full time · Hybrid · 2 openings · intake 1 Nov 2026")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit budget" })).toBeTruthy();
  });

  it("says the budget is restricted, without edit buttons, for other roles", async () => {
    const restricted: Partial<JobOrder> = { ...job, restrictedFields: ["salary"] };
    delete restricted.salary; // left out, as the API does
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok(restricted))) as never);
    renderWithClient(<JobDetail id={job.id} permissions={["jobs:read"]} />);
    await screen.findByRole("heading", { name: /Senior Auditor/ });
    expect(screen.getByText(/restricted for your role/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /budget|Edit job/ })).toBeNull();
  });
});
