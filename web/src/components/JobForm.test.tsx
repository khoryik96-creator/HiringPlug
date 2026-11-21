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
    expect(sentBody(post)).not.toHaveProperty("jdFileId"); // no job description dropped
  });

  it("is filled in from a job description dropped on it, keeping what was typed; the file is kept with the job", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok({ items: [], nextCursor: null }))) as never);
    const details = { title: "Finance Manager", location: "Petaling Jaya, Selangor", country: "MY", workType: "full_time", workArrangement: "hybrid",
      salaryText: "RM 12,000", closesOn: null, description: "Lead month-end closing." };
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: "jdf_1", jobId: null, fileName: "JD.txt", status: "read", error: null, details, createdAt: "2026-11-07T00:00:00Z",
    }), { status: 201 }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(job));
    renderWithClient(<JobForm job={null} company={{ id: job.companyId, name: "Maybank" }} onDone={vi.fn()} />);
    change("Location", "Kuala Lumpur");
    const drop = screen.getByLabelText(/Drop the client's JD/);
    fireEvent.change(drop, { target: { files: [new File(["Position: Finance Manager"], "JD.txt", { type: "text/plain" })] } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Job title").value).toBe("Finance Manager"); });
    expect(fetch.mock.calls[0]?.[0]).toBe("/v1/jd-files");
    expect(screen.getByLabelText<HTMLInputElement>("Location").value).toBe("Kuala Lumpur");
    expect(screen.getByLabelText<HTMLSelectElement>("Where").value).toBe("hybrid");
    expect(screen.getByLabelText<HTMLTextAreaElement>("Description").value).toBe("Lead month-end closing.");
    fireEvent.click(screen.getByRole("button", { name: "Add job" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ title: "Finance Manager", jdFileId: "jdf_1" });
  });

  it("takes a second JD dropped over the first: what the first filled is replaced, what was typed stays", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok({ items: [], nextCursor: null }))) as never);
    const read = (id: string, details: Record<string, unknown>) => new Response(JSON.stringify({
      id, jobId: null, fileName: `${id}.txt`, status: "read", error: null, details: { salaryText: null, closesOn: null, ...details }, createdAt: "2026-11-07T00:00:00Z",
    }), { status: 201 });
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(read("jdf_1", { title: "Finance Manager", location: "Petaling Jaya", country: "MY", workType: "full_time", workArrangement: "hybrid", description: "Lead month-end closing.", openings: 2 }))
      .mockResolvedValueOnce(read("jdf_2", { title: "Tax Manager", location: null, country: "SG", workType: "contract", workArrangement: null, description: "Run the tax team.", openings: null }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(job));
    renderWithClient(<JobForm job={null} company={{ id: job.companyId, name: "Maybank" }} onDone={vi.fn()} />);
    const drop = () => screen.getByLabelText(/Drop the client's JD/);
    fireEvent.change(drop(), { target: { files: [new File(["a"], "a.txt", { type: "text/plain" })] } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Job title").value).toBe("Finance Manager"); });
    change("Description", "Our own words."); // typed after the first JD
    fireEvent.change(drop(), { target: { files: [new File(["b"], "b.txt", { type: "text/plain" })] } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Job title").value).toBe("Tax Manager"); });
    expect(screen.getByLabelText<HTMLInputElement>("Location").value).toBe(""); // the first JD's, which the second does not say
    expect(screen.getByLabelText<HTMLSelectElement>("Work type").value).toBe("contract");
    expect(screen.getByLabelText<HTMLSelectElement>("Where").value).toBe("");
    expect(screen.getByLabelText<HTMLInputElement>("Openings").value).toBe("1");
    expect(screen.getByLabelText<HTMLTextAreaElement>("Description").value).toBe("Our own words.");
    fireEvent.click(screen.getByRole("button", { name: "Add job" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ title: "Tax Manager", jdFileId: "jdf_2" });
  });

  it("chooses the company the JD names when it is on record, and takes how many are wanted", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
      path === "/v1/settings/fields" ? ok(testFieldSettings())
        : path === "/v1/companies" ? ok({ items: [{ id: "comp_maju", name: "Maju Foods Bhd", city: "Shah Alam" }], nextCursor: null })
          : ok({ items: [], nextCursor: null }))) as never);
    const details = { title: "Senior Production Planner", location: "Shah Alam", country: "MY", workType: "full_time", workArrangement: "onsite",
      salaryText: null, closesOn: null, description: "Plan production.", companyName: "MAJU FOODS BERHAD", openings: 2 };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: "jdf_2", jobId: null, fileName: "JD.pdf", status: "read", error: null, details, createdAt: "2026-11-07T00:00:00Z",
    }), { status: 201 }));
    renderWithClient(<JobForm job={null} onDone={vi.fn()} />);
    fireEvent.change(screen.getByLabelText(/Drop the client's JD/), { target: { files: [new File(["x"], "JD.txt", { type: "text/plain" })] } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Openings").value).toBe("2"); });
    await waitFor(() => { expect(screen.getByText("Maju Foods Bhd")).toBeTruthy(); });
    expect(get).toHaveBeenCalledWith("/v1/companies", { params: { query: { q: "maju foods", limit: 8 } } });
  });

  it("names the JD's company when none on record has that name", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
      path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok({ items: [], nextCursor: null }))) as never);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: "jdf_3", jobId: null, fileName: "JD.pdf", status: "read", error: null, createdAt: "2026-11-07T00:00:00Z",
      details: { title: "Planner", location: null, country: null, workType: null, workArrangement: null, salaryText: null, closesOn: null,
        description: "", companyName: "Contoh Holdings Sdn Bhd", openings: null },
    }), { status: 201 }));
    renderWithClient(<JobForm job={null} onDone={vi.fn()} />);
    fireEvent.change(screen.getByLabelText(/Drop the client's JD/), { target: { files: [new File(["x"], "JD.txt", { type: "text/plain" })] } });
    expect(await screen.findByText(/The JD names “Contoh Holdings Sdn Bhd”/)).toBeTruthy();
    expect(screen.getByLabelText<HTMLInputElement>("Openings").value, "the default stays when the JD does not say").toBe("1");
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
    expect(screen.getByRole("tab", { name: "Notes & activity" }).getAttribute("aria-selected")).toBe("true"); // the rest in tabs
    expect(screen.queryByText("MYR 9000.00 – 12000.00 / month")).toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: "Budget & retainer" }));
    expect(screen.getByText("MYR 9000.00 – 12000.00 / month")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit budget" })).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Job details" }));
    expect(screen.getByText("Kuala Lumpur, Malaysia (MY) · Full time · Hybrid · 2 openings · intake 1 Nov 2026")).toBeTruthy();
  });

  it("says the budget is restricted, without edit buttons, for other roles", async () => {
    const restricted: Partial<JobOrder> = { ...job, restrictedFields: ["salary"] };
    delete restricted.salary; // left out, as the API does
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok(restricted))) as never);
    renderWithClient(<JobDetail id={job.id} permissions={["jobs:read"]} />);
    await screen.findByRole("heading", { name: /Senior Auditor/ });
    fireEvent.click(screen.getByRole("tab", { name: "Budget & retainer" }));
    expect(screen.getByText(/restricted for your role/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /budget|Edit job/ })).toBeNull();
  });
});
