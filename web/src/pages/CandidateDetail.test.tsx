import { cleanup, within, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Candidate, type Permission } from "../api/client";
import { candidateKey } from "../api/keys";
import { failure, ok, RECRUITER, renderWithClient, sentBody, testCandidate, testFieldSettings } from "../test-utils";
import { CandidateDetail } from "./CandidateDetail";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
});

/** Answers the page's reads: the candidate (or `candidateResult`) and an empty document list. */
function mockReads(candidateResult: () => unknown) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/candidates/{candidateId}") return Promise.resolve(candidateResult());
    if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: [] }));
    return Promise.reject(new Error(`unexpected GET ${path}`));
  }) as never);
}

async function show(candidate: Candidate, permissions: Permission[]) {
  mockReads(() => ok(candidate));
  const view = renderWithClient(<CandidateDetail id={candidate.id} permissions={permissions} />);
  await screen.findByRole("heading", { name: candidate.fullName });
  return view;
}

// Each button's name: an icon button's is its label (Edit profile is a pencil).
const buttons = () => screen.queryAllByRole("button").map((b) => b.getAttribute("aria-label") ?? b.textContent);

describe("candidate page", () => {
  it("shows its actions as named icons, each with a tooltip", async () => {
    await show(testCandidate(), RECRUITER);
    const bar = screen.getByRole("toolbar", { name: "Candidate actions" });
    for (const name of ["Edit profile"]) {
      const b = within(bar).getByRole("button", { name });
      expect(b.textContent).toBe(""); // an icon, not a word
      expect(b.getAttribute("data-tip")).toBe(name);
    }
    expect(within(bar).getByRole("link", { name: "Screening call" }).getAttribute("href")).toContain("/screening");
  });


  it("explains when a saved screening call kept the later salary", async () => {
    const c = testCandidate();
    window.location.hash = `#/candidates/${c.id}?screeningSalary=kept`;
    await show(c, RECRUITER);
    expect(screen.getByRole("status").textContent).toContain("A later call already set the salary");
  });
  it("opens Details first and exposes only permitted workspaces", async () => {
    await show(testCandidate(), ["candidates:read", "documents:read"]);
    expect(screen.getAllByRole("tab").map(tab => tab.textContent)).toEqual(["Details", "CV & files", "Activity", "Salary"]);
    expect(screen.getByRole("tab", {name: "Details"}).getAttribute("aria-selected")).toBe("true");
    expect(screen.queryByRole("region", {name: "Documents"})).toBeNull();
    fireEvent.click(screen.getByRole("tab", {name: "CV & files"}));
    expect(await screen.findByRole("region", {name: "Documents"})).toBeTruthy();
  });

  it("shows the CV beside the details on request, remembers it, and not on the CV tab", async () => {
    localStorage.removeItem("hp.cvBeside");
    await show(testCandidate(), ["candidates:read", "documents:read"]);
    const toggle = screen.getByRole("button", { name: "CV beside" });
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    expect(screen.queryByRole("region", { name: "CV beside the details" })).toBeNull();
    fireEvent.click(toggle);
    expect(await screen.findByText("No CV yet. Add one in CV & files.")).toBeTruthy();
    expect(screen.getByRole("region", { name: "Profile" }), "the details stay").toBeTruthy();
    expect(localStorage.getItem("hp.cvBeside")).toBe("on");
    fireEvent.click(screen.getByRole("tab", { name: "CV & files" }));
    expect(screen.queryByRole("region", { name: "CV beside the details" }), "the CV tab shows it already").toBeNull();
    expect(screen.queryByRole("button", { name: "CV beside" }), "no switch that would do nothing there").toBeNull();
    cleanup();
    await show(testCandidate(), ["candidates:read", "documents:read"]);
    expect(screen.getByRole("region", { name: "CV beside the details" }), "remembered").toBeTruthy();
    localStorage.removeItem("hp.cvBeside");
  });
  it("offers no CV beside to a role that may not see CVs", async () => {
    await show(testCandidate(), ["candidates:read"]);
    expect(screen.queryByRole("button", { name: "CV beside" })).toBeNull();
  });

  it("lists industries and tags in one column, under the specialization", async () => {
    await show(testCandidate(), ["candidates:read"]);
    const profile = screen.getByRole("region", { name: "Profile" });
    expect([...profile.querySelectorAll("dt")].map((d) => d.textContent)).toEqual(["Work authorization", "Specialization", "Industries", "IT skills", "Certifications", "Tags", "Source"]);
    expect(profile.className, "one column on the candidate's page only (companies and contacts keep theirs)").toContain("one-column");
  });
  it("opens the CV workspace for a search highlight link", async () => {
    window.location.hash = `#/candidates/${testCandidate().id}?hl=Playwright`;
    await show(testCandidate(), ["candidates:read", "documents:read"]);
    expect(screen.getByRole("tab", {name: "CV & files"}).getAttribute("aria-selected")).toBe("true");
  });

  it("gives viewers no edit buttons", async () => {
    await show(testCandidate(), ["candidates:read", "candidates.compensation:read", "documents:read"]);
    expect(buttons()).not.toContain("Edit profile");
    expect(buttons()).not.toContain("Add salary");
  });

  it("lets recruiters edit the profile and the salary", async () => {
    await show(testCandidate(), RECRUITER);
    expect(buttons()).toContain("Edit profile");
    fireEvent.click(screen.getByRole("tab", {name: "Salary"}));
    expect(buttons()).toContain("Add salary");
  });

  it("opens the salary form from the card's + Add (the profile's form has no salary)", async () => {
    await show(testCandidate({ compensation: null }), RECRUITER);
    const current = screen.getAllByText("Current salary").map((el) => el.closest(".fact")).find(Boolean) as HTMLElement;
    fireEvent.click(within(current).getByRole("button", { name: "+ Add" }));
    expect(screen.getByRole("tab", { name: "Salary" }).getAttribute("aria-selected")).toBe("true");
    expect(await screen.findByLabelText(/Current salary|Amount/)).toBeTruthy();
    expect(buttons()).not.toContain("Save profile");
  });

  it("changes a salary already recorded from the card", async () => {
    await show(testCandidate({ compensation: {
      version: 1, currentSalary: { amount: "9500.00", currency: "MYR", period: "month" }, expectedSalary: null, salaryNotes: null, updatedAt: "2026-11-01T00:00:00Z",
    } as never }), RECRUITER);
    fireEvent.click(screen.getByRole("button", { name: "Change current salary" }));
    expect(screen.getByRole("tab", { name: "Salary" }).getAttribute("aria-selected")).toBe("true");
    expect(buttons()).not.toContain("Edit salary"); // the form is open
  });

  it("lets sourcers edit the profile, but not see or edit salary", async () => {
    await show(testCandidate({ restrictedFields: ["compensation"] }), ["candidates:read", "candidates:update", "documents:read"]);
    expect(buttons()).toContain("Edit profile");
    fireEvent.click(screen.getByRole("tab", {name: "Salary"}));
    expect(screen.getByText(/restricted for your role/)).toBeTruthy();
    expect(buttons()).not.toContain("Add salary");
  });

  it("keeps an open form and what was typed when a background refresh fails", async () => {
    const candidate = testCandidate();
    let next: unknown = ok(candidate);
    mockReads(() => next);
    const { client } = renderWithClient(<CandidateDetail id={candidate.id} permissions={RECRUITER} />);
    fireEvent.click(await screen.findByRole("button", { name: "Edit profile" }));
    fireEvent.change(screen.getByLabelText("Current title"), { target: { value: "Senior Accountant" } });

    next = failure("SERVICE_UNAVAILABLE", "The service is unavailable. Try again shortly.", undefined, 503);
    await client.refetchQueries({ queryKey: candidateKey(candidate.id) });

    expect(await screen.findByText("Showing the version loaded earlier.")).toBeTruthy();
    expect(screen.getByLabelText<HTMLInputElement>("Current title").value).toBe("Senior Accountant");
  });

  it("shows nationality, source and work authorization in words", async () => {
    await show(
      testCandidate({ nationality: "MY", source: "careers_page", workAuthorization: { status: "expat_eligible", basis: "rp_t" } }),
      RECRUITER,
    );
    const banner = screen.getByRole("region", { name: "Candidate" });
    expect(within(banner).getByText("Malaysia")).toBeTruthy();
    // The source is under Details now, not on the card (its contact plate has email, phone and LinkedIn).
    expect(within(banner).queryByText("Careers page")).toBeNull();
    expect(within(screen.getByRole("region", {name: "Profile"})).getByText("Careers page")).toBeTruthy();
    expect(banner.textContent).toContain("Work authorization: RP-T"); // the column on the right
    expect(within(screen.getByRole("region", {name: "Profile"})).getByRole("button", {name: "Spousal / RP-T"})).toBeTruthy();
  });
});

describe("possible duplicates", () => {
  it("warns about candidates with the same phone or a similar name", async () => {
    const similar = { items: [{ candidate: { id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Siti Aishah", email: null, phone: null, currentTitle: "Accountant", currentCompany: null, location: null, workAuthorizationStatus: null, updatedAt: "2026-11-01T00:00:00Z" }, reasons: ["similar_name"] }] };
    vi.spyOn(api, "GET").mockImplementation(((path: string) => {
      if (path === "/v1/candidates/{candidateId}/similar") return Promise.resolve(ok(similar));
      if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(testCandidate()));
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      return Promise.resolve(ok({ items: [] }));
    }));
    renderWithClient(<CandidateDetail id={testCandidate().id} permissions={RECRUITER} />);
    const warning = await screen.findByRole("region", { name: "Possible duplicates" });
    expect(warning.textContent).toContain("1 possible duplicate · similar name");
    expect(warning.textContent).not.toContain("Siti Aishah"); // one line until opened
    fireEvent.click(within(warning).getByRole("button", { name: "Review" }));
    expect(warning.textContent).toContain("Siti Aishah");
    expect(warning.textContent).toContain("similar name; Accountant");
  });

  it("merges a duplicate into this candidate after confirming", async () => {
    const similar = { items: [{ candidate: { id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Siti Aishah", email: null, phone: null, currentTitle: null, currentCompany: null, location: null, workAuthorizationStatus: null, updatedAt: "2026-11-01T00:00:00Z" }, reasons: ["same_phone"] }] };
    vi.spyOn(api, "GET").mockImplementation(((path: string) => {
      if (path === "/v1/candidates/{candidateId}/similar") return Promise.resolve(ok(similar));
      if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(testCandidate()));
      if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
      return Promise.resolve(ok({ items: [] }));
    }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(testCandidate({ version: 4 })));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<CandidateDetail id={testCandidate().id} permissions={[...RECRUITER, "candidates:merge"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Review and merge" }));
    fireEvent.click(screen.getByRole("button", { name: "Merge into this one" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ duplicateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", version: 3 });
  });
});

describe("the candidate's card", () => {
  it("has email, phone and LinkedIn under where they live; clicking the email or number copies it", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    await show(testCandidate({ email: "farah@example.com", phone: "+60123456789", linkedinUrl: "https://www.linkedin.com/in/farah" }), RECRUITER);
    const plate = within(screen.getByRole("region", { name: "Candidate" })).getByLabelText("Contact");
    expect([...plate.querySelectorAll("dt")].map((d) => d.textContent)).toEqual(["Nationality", "Lives in", "Email", "Phone", "LinkedIn"]);
    fireEvent.click(within(plate).getByRole("button", { name: "farah@example.com: copy email" }));
    expect(await within(plate).findByText("Copied")).toBeTruthy();
    expect(writeText).toHaveBeenCalledWith("farah@example.com");
    fireEvent.click(within(plate).getByRole("button", { name: "+60123456789: copy phone number" }));
    await waitFor(() => { expect(writeText).toHaveBeenLastCalledWith("+60123456789"); });
    expect(within(plate).getByRole("link", { name: "Profile ↗" }).getAttribute("href")).toBe("https://www.linkedin.com/in/farah");
    expect(screen.queryByRole("complementary", { name: "Contact summary" }), "no second contact box").toBeNull();
  });
});
