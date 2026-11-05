import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import * as files from "../api/files";
import { invalidField, ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { AdVersions, exportName } from "./AdVersions";
import { formFromJob, jobFields, todayLocal } from "./jobFormState";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const ad: Schemas["JobAd"] = {
  id: "ad_1", version: 3, jobId: "jo_1", title: "Ops Manager at Maju", location: "Maju hub, Shah Alam", country: "MY", workType: "full_time",
  workArrangement: null, salaryText: null, description: "Maju is growing.", closesOn: null, status: "published",
  publishedAt: "2026-11-01T00:00:00Z", createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  blindTitle: "Ops Manager at our client", blindDescription: "A logistics firm is growing.", clientDescription: "a logistics firm",
  blindLocation: "Shah Alam", blindSalaryText: null,
  blindEdited: true, blindOutdated: true, publishBlind: true,
};

describe("ad versions", () => {
  it("shows the ad as written and its blind version side by side", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    renderWithClient(<AdVersions ad={ad} canEdit />);
    const written = screen.getByRole("region", { name: "As written" });
    const blind = screen.getByRole("region", { name: "Blind" });
    expect(written.textContent).toContain("Maju is growing.");
    expect(written.textContent).toContain("Maju hub, Shah Alam");
    expect(blind.textContent).not.toContain("Maju");
    expect(written.textContent).not.toContain("On the careers page");
    expect(blind.textContent).toContain("On the careers page");
    expect(blind.textContent).toContain("A logistics firm is growing.");
    expect(await within(blind).findByText(/Shah Alam, Malaysia \(MY\) · Full time · your own text/)).toBeTruthy();
    expect(within(blind).getByRole("status").textContent).toContain("The ad changed after this blind text was written");
  });

  it("saves a person's blind text, or makes it again from the ad", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValueOnce(invalidField("blindDescription", "still names the client"))
      .mockResolvedValue(ok(ad));
    renderWithClient(<AdVersions ad={ad} canEdit />);
    fireEvent.click(screen.getByRole("button", { name: "Edit blind ad" }));
    fireEvent.change(screen.getByLabelText("Blind text"), { target: { value: "Join Maju!" } });
    fireEvent.click(screen.getByRole("button", { name: "Save blind ad" }));
    expect(await screen.findByText("still names the client")).toBeTruthy();
    expect(sentBody(patch)).toEqual({ version: 3, blindTitle: "Ops Manager at our client", blindDescription: "Join Maju!" });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Make again from the ad" }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
    expect(sentBody(patch, 1)).toEqual({ version: 3, remakeBlind: true });
  });

  it("downloads either version as Word or PDF", () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    const download = vi.spyOn(files, "downloadFrom").mockResolvedValue();
    renderWithClient(<AdVersions ad={ad} canEdit={false} />);
    fireEvent.click(within(screen.getByRole("region", { name: "Blind" })).getByRole("button", { name: "PDF" }));
    expect(download).toHaveBeenCalledWith("/v1/job-ads/ad_1/export?version=blind&format=pdf", "Ops Manager at our client (blind).pdf", "application/pdf");
    expect(screen.queryByRole("button", { name: "Edit blind ad" })).toBeNull();
    expect(exportName("C++ / Developer", "original", "docx")).toBe("C Developer.docx");
  });
});

describe("job form", () => {
  it("dates a new job's intake today, and sends its country", () => {
    const form = { ...formFromJob(null), title: "Clerk", country: "SG" };
    expect(form.intakeDate).toBe(todayLocal());
    expect(jobFields(form)).toMatchObject({ country: "SG", intakeDate: todayLocal() });
    expect(todayLocal(new Date(2026, 0, 5))).toBe("2026-01-05");
  });
});
