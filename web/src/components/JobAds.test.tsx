import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type JobOrder, type Schemas } from "../api/client";
import { failure, ok, RECRUITER, renderWithClient, sentBody, testMe } from "../test-utils";
import { AdText } from "./AdText";
import { JobAds } from "./JobAds";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const job: JobOrder = {
  id: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 2, companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank",
  contactId: null, contactName: null, title: "Senior Auditor", status: "open", location: "Kuala Lumpur",
  workType: "full_time", workArrangement: "hybrid", openings: 2, description: "Audit banks.",
  restrictedFields: ["salary"], ownerId: null, ownerName: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};
/** Answers the panel's reads: who is signed in (for the careers link), and the ads. */
const reads = (ads: Schemas["JobAd"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(path === "/v1/me" ? ok(testMe(RECRUITER)) : ok({ items: ads }))) as never);

const draft: Schemas["JobAd"] = {
  id: "ad_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, jobId: job.id, title: "Senior Auditor", location: "Kuala Lumpur",
  workType: "full_time", workArrangement: null, salaryText: "RM 9,000", description: null, closesOn: null, status: "draft",
  publishedAt: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

describe("ad text", () => {
  it("shows paragraphs and bullet points, and never runs HTML", () => {
    const { container } = render(<AdText text={"We are hiring.\n\nYou bring:\n- SAP FICO\n• Excel\n<img src=x onerror=alert(1)>"} />);
    expect([...container.querySelectorAll("li")].map((li) => li.textContent)).toEqual(["SAP FICO", "Excel"]);
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
  });
});

describe("job ads", () => {
  it("starts a new ad from the job's own details", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(draft));
    renderWithClient(<JobAds job={job} canEdit />);
    fireEvent.click(await screen.findByRole("button", { name: "Write an ad" }));
    fireEvent.change(screen.getByLabelText("Salary shown on the ad"), { target: { value: "RM 9,000 - 12,000" } });
    fireEvent.click(screen.getByRole("button", { name: "Save as draft" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({
      title: "Senior Auditor", location: "Kuala Lumpur", workType: "full_time", workArrangement: "hybrid",
      salaryText: "RM 9,000 - 12,000", closesOn: null, description: "Audit banks.",
    });
  });

  it("publishes with the ad's version and says why when it cannot", async () => {
    reads([draft]);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(
      failure("JOB_AD_NOT_PUBLISHABLE", "Write the ad's text before publishing it.", undefined, 422),
    );
    renderWithClient(<JobAds job={job} canEdit />);
    const ad = await screen.findByRole("article", { name: "Senior Auditor" });
    fireEvent.click(within(ad).getByRole("button", { name: "Publish" }));
    expect(await screen.findByText("Write the ad's text before publishing it.")).toBeTruthy();
    expect(sentBody(patch)).toEqual({ version: 1, status: "published" });
  });

  it("gives viewers no buttons", async () => {
    reads([draft]);
    renderWithClient(<JobAds job={job} canEdit={false} />);
    await screen.findByRole("article", { name: "Senior Auditor" });
    expect((await screen.findByRole("link", { name: /\/careers\/demo-company$/ })).getAttribute("href")).toBe(`${window.location.origin}/careers/demo-company`);
    expect(screen.queryAllByRole("button")).toEqual([]);
  });

  it("gives published ads a link per job board", async () => {
    reads([{ ...draft, status: "published", publishedAt: "2026-11-01T00:00:00Z", description: "Text." }]);
    const copy = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText: copy } });
    renderWithClient(<JobAds job={job} canEdit />);
    const links = await screen.findByText("Links for job boards");
    fireEvent.click(links);
    const row = screen.getByText("LinkedIn").closest("tr");
    expect(row?.textContent).toContain(`/careers/demo-company/${draft.id}?via=linkedin`);
    fireEvent.click(within(row as HTMLElement).getByRole("button", { name: "Copy" }));
    await waitFor(() => { expect(copy).toHaveBeenCalledWith(`${window.location.origin}/careers/demo-company/${draft.id}?via=linkedin`); });
    vi.unstubAllGlobals();
  });
});
