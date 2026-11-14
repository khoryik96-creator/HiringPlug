import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type JobOrder } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { JobStatusSelect } from "./JobStatusSelect";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const job = { id: "job_1", title: "Plant Manager", status: "open", version: 3 } as JobOrder;

describe("a job's status", () => {
  it("is changed from a dropdown: open, on hold, filled or closed", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...job, status: "on_hold", version: 4 }));
    renderWithClient(<JobStatusSelect job={job} canChange />);
    const select = screen.getByLabelText("Job status");
    expect([...select.querySelectorAll("option")].map((o) => o.textContent)).toEqual(["Open", "On hold", "Filled", "Closed"]);
    fireEvent.change(select, { target: { value: "on_hold" } });
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(patch.mock.calls[0]?.[0]).toBe("/v1/jobs/{jobId}");
    expect(sentBody(patch)).toEqual({ version: 3, status: "on_hold" });
  });

  it("shows the choice while it saves", async () => {
    let answer: (v: never) => void = () => undefined;
    vi.spyOn(api, "PATCH").mockReturnValue(new Promise((r) => { answer = r; }) as never);
    renderWithClient(<JobStatusSelect job={job} canChange />);
    const select = screen.getByLabelText<HTMLSelectElement>("Job status");
    fireEvent.change(select, { target: { value: "closed" } });
    await waitFor(() => { expect(select.disabled).toBe(true); });
    expect(select.value).toBe("closed");
    answer(ok({ ...job, status: "closed", version: 4 }));
  });

  it("says why a change did not save", async () => {
    vi.spyOn(api, "PATCH").mockResolvedValue(failure("CONFLICT_STALE_VERSION", "Someone else changed this job. Reload it.", undefined, 409));
    renderWithClient(<JobStatusSelect job={job} canChange />);
    fireEvent.change(screen.getByLabelText("Job status"), { target: { value: "closed" } });
    expect(await screen.findByText("Someone else changed this job. Reload it.")).toBeTruthy();
  });

  it("is only a badge without jobs:update", () => {
    renderWithClient(<JobStatusSelect job={job} canChange={false} />);
    expect(screen.queryByLabelText("Job status")).toBeNull();
    expect(screen.getByText("Open")).toBeTruthy();
  });
});
