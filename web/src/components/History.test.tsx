import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate } from "../test-utils";
import { History } from "./History";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("work history", () => {
  const experience = [
    { title: "Senior Accountant", company: "Contoh Berhad", startDate: "2020-01", endDate: null, current: true, description: null },
  ];

  it("shows jobs with their periods, and no edit button without permission", () => {
    renderWithClient(<History candidate={testCandidate({ experience })} canEdit={false} />);
    expect(screen.getByText("Senior Accountant")).toBeTruthy();
    expect(screen.getByText("Jan 2020 – Present")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Edit history" })).toBeNull();
  });

  it("edits the whole history and saves it with the version it was opened with", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(testCandidate({ version: 4 })));
    renderWithClient(<History candidate={testCandidate({ experience, version: 3 })} canEdit />);
    fireEvent.click(screen.getByRole("button", { name: "Edit history" }));
    fireEvent.click(screen.getByRole("button", { name: "Add job" }));
    const added = screen.getByRole("group", { name: "Job 2" });
    fireEvent.change(within(added).getByLabelText("Job title"), { target: { value: "Accountant" } });
    fireEvent.change(within(added).getByLabelText("From (YYYY-MM)"), { target: { value: "2016-03" } });
    fireEvent.change(within(added).getByLabelText("To (YYYY-MM)"), { target: { value: "2019-12" } });
    fireEvent.click(screen.getByRole("button", { name: "Add qualification" }));
    fireEvent.change(screen.getByLabelText("Qualification"), { target: { value: "SPM" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({
      version: 3,
      experience: [
        experience[0],
        { title: "Accountant", company: null, startDate: "2016-03", endDate: "2019-12", current: false, description: null },
      ],
      education: [{ qualification: "SPM", institution: null, endYear: null }],
    });
  });
});
