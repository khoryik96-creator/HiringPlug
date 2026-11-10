import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type JobOrder, type Schemas } from "../api/client";
import { failure, ok, RECRUITER, renderWithClient, sentBody, testFieldSettings, testMe } from "../test-utils";
import { AdText } from "./AdText";
import { JobAds } from "./JobAds";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const job: JobOrder = {
  id: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 2, companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank",
  contactId: null, contactName: null, title: "Senior Auditor", status: "open", location: "Kuala Lumpur",
  workType: "full_time", workArrangement: "hybrid", openings: 2, description: "Audit banks.", country: "MY", intakeDate: "2026-11-01",
  restrictedFields: ["salary"], ownerId: null, ownerName: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};
/** Answers the panel's reads: who is signed in (for the careers link), and the ads. */
const reads = (ads: Schemas["JobAd"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(path === "/v1/me" ? ok(testMe(RECRUITER)) : path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok({ items: ads }))) as never);

const draft: Schemas["JobAd"] = {
  id: "ad_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, jobId: job.id, title: "Senior Auditor", location: "Kuala Lumpur",
  workType: "full_time", workArrangement: null, salaryText: "RM 9,000", description: null, closesOn: null, status: "draft",
  publishedAt: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  country: "MY", blindTitle: "Senior Auditor", blindDescription: null, blindLocation: "Kuala Lumpur", blindSalaryText: "RM 9,000", clientDescription: null,
  blindEdited: false, blindOutdated: false, publishBlind: true,
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
      country: "MY", clientDescription: null, publishBlind: true, // the job's country; new ads go out blind
    });
  });

  it("writes an ad from a job description dropped on the panel; a PDF is read first", async () => {
    const details: Schemas["JdDetails"] = {
      title: "Finance Manager", location: "Petaling Jaya", country: "MY", workType: "contract", workArrangement: "remote",
      salaryText: "RM 12,000 - 15,000 per month", closesOn: "2026-12-31", description: "Maju Foods needs a Finance Manager.\n- Lead closing",
    };
    const jdFile = { id: "jdf_01jbz6w0m0k8q9x2r3s4t5v6w7", jobId: job.id, fileName: "JD.pdf", error: null, createdAt: "x" };
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
      path === "/v1/me" ? ok(testMe(RECRUITER)) : path === "/v1/settings/fields" ? ok(testFieldSettings())
        : path === "/v1/jd-files/{jdFileId}" ? ok({ ...jdFile, status: "read", details }) : ok({ items: [] }),
    )) as never);
    const send = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ...jdFile, status: "reading", details: null }), { status: 201 }));
    vi.stubGlobal("fetch", send);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(draft));
    renderWithClient(<JobAds job={job} canEdit />);

    const zone = (await screen.findByText(/Drop the client's JD to write an ad/)).closest(".drop-zone");
    if (!zone) throw new Error("no drop zone");
    fireEvent.drop(zone, { dataTransfer: { files: [new File(["%PDF"], "JD.pdf")] } });
    const form = await screen.findByRole("form", { name: "New ad" });
    expect(send.mock.calls[0]?.[0]).toBe(`/v1/jobs/${job.id}/jd-files`);
    await waitFor(() => { expect(within(form).getByLabelText<HTMLInputElement>("Ad title").value).toBe("Finance Manager"); }, { timeout: 4000 });
    expect(get).toHaveBeenCalledWith("/v1/jd-files/{jdFileId}", { params: { path: { jdFileId: jdFile.id } } });
    expect(within(form).getByText(/Filled in from JD.pdf/)).toBeTruthy();

    fireEvent.click(within(form).getByRole("button", { name: "Save as draft" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({
      title: "Finance Manager", location: "Petaling Jaya", country: "MY", workType: "contract", workArrangement: "remote",
      salaryText: "RM 12,000 - 15,000 per month", closesOn: "2026-12-31", description: "Maju Foods needs a Finance Manager.\n- Lead closing",
      clientDescription: null, publishBlind: true,
    });
    vi.unstubAllGlobals();
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
    expect((await screen.findByRole("link", { name: "Careers page" })).getAttribute("href")).toBe(`${window.location.origin}/careers/demo-company`);
    // They may fold the panel and the ad, pick a version to read, and download the ad (Word, PDF); not change it.
    expect(screen.queryAllByRole("button").map((b) => b.textContent)).toEqual(
      ["Job ads", "Senior Auditor", "Side by side", "As written", "Blind", "Word", "PDF", "Word", "PDF"],
    );
  });

  it("folds into a card per ad: the newest open, older ones closed; one version at a time if wanted", async () => {
    const older = { ...draft, id: "ad_01jbz6w0m0k8q9x2r3s4t5v6w0", title: "Auditor (old)" };
    reads([draft, older]);
    renderWithClient(<JobAds job={job} canEdit={false} />);
    const newest = await screen.findByRole("article", { name: "Senior Auditor" });
    const old = screen.getByRole("article", { name: "Auditor (old)" });
    expect(within(newest).getByRole("region", { name: "Blind" })).toBeTruthy();
    expect(within(old).queryByRole("region", { name: "Blind" })).toBeNull();
    expect(screen.getByText("2 ads · 0 on the careers page")).toBeTruthy();

    fireEvent.click(within(newest).getByRole("button", { name: "Blind" }));
    expect(within(newest).queryByRole("region", { name: "As written" })).toBeNull();
    expect(within(newest).getByRole("region", { name: "Blind" })).toBeTruthy();

    fireEvent.click(within(old).getByRole("button", { name: "Auditor (old)" }));
    expect(within(old).getByRole("region", { name: "As written" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Job ads" }));
    expect(screen.queryByRole("article")).toBeNull();
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
