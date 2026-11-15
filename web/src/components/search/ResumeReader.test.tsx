import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { ok, renderWithClient } from "../../test-utils";
import { ResumeReader } from "./ResumeReader";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const person = (id: string, fullName: string, more: object = {}) => ({
  id, fullName, email: null, phone: null, currentTitle: "Accountant", currentCompany: "Maju Foods", location: null,
  workAuthorizationStatus: "local", tags: [], updatedAt: "2026-11-01T00:00:00Z", availability: null, availabilitySince: null,
  noticePeriod: { value: 1, unit: "month" }, currentSalary: { amount: "8000", currency: "MYR", period: "month" },
  expectedSalary: { min: "9500", currency: "MYR", period: "month" }, salaryRestricted: false,
  workAuthorizationBasis: null, nationality: null, residenceCountry: null, source: null, industry: null, ...more,
});
const hit = (c: ReturnType<typeof person>, matchedIn: ("cv" | "profile")[] = ["cv"]) => ({ candidate: c, matchedIn, cvSnippet: null });
const nurul = person("cand_1", "Nurul Aisyah");
const wei = person("cand_2", "Wei Tan", { salaryRestricted: true, currentSalary: null, expectedSalary: null });
const siti = person("cand_3", "Siti Hajar");

/** Each candidate has one CV, whose text names them. */
function serveCvs() {
  return vi.spyOn(api, "GET").mockImplementation(((path: string, init: { params: { path: { candidateId?: string; documentId?: string } } }) => {
    if (path === "/v1/candidates/{candidateId}/documents") {
      const id = init.params.path.candidateId ?? "";
      return Promise.resolve(ok({ items: id === "cand_3" ? [] : [cv(id)] }));
    }
    if (path === "/v1/documents/{documentId}") return Promise.resolve(ok(cv((init.params.path.documentId ?? "").replace("doc_", ""))));
    if (path === "/v1/jobs") return Promise.resolve(ok({ items: [{ id: "job_1", title: "Finance Manager", companyName: "Acme" }] }));
    return Promise.resolve(ok({}));
  }) as never);
}
const cv = (candidateId: string) => ({
  id: `doc_${candidateId}`, candidateId, source: "original", fileName: `cv-${candidateId}.txt`, contentType: "text/plain", sizeBytes: 10,
  sha256: "0".repeat(64), status: "extracted", pageCount: 1, extractor: "x", errorMessage: null, extractionWarnings: [], parseStatus: "parsed",
  extractedText: `the CV of ${candidateId}`, parsedProfile: null, pdfStatus: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
});

function reader(more: Partial<Parameters<typeof ResumeReader>[0]> = {}) {
  const seen = new Set<string>();
  const props = {
    hits: [hit(nurul), hit(wei, ["profile"]), hit(siti, [])] as never, highlight: "", profileQuery: "?hl=finance",
    seen, onSeen: (id: string) => { seen.add(id); }, canAddToJob: false, ...more,
  };
  return { ...renderWithClient(<ResumeReader {...props} />), seen };
}

describe("reading search results as resumes", () => {
  it("opens the first result's CV, with their pay and notice, and a link to their profile", async () => {
    serveCvs();
    reader();
    const pane = screen.getByRole("region", { name: "Resume of Nurul Aisyah" });
    expect(await screen.findByText("the CV of cand_1")).toBeTruthy();
    expect(pane.textContent).toContain("RM 8,000");
    expect(pane.textContent).toContain("RM 9,500");
    expect(pane.textContent).toContain("1 month");
    expect(screen.getByRole("link", { name: "Open profile" }).getAttribute("href")).toBe("#/candidates/cand_1?hl=finance");
    expect(screen.getByRole("status").textContent).toBe("1 of 3");
    const first = screen.getByRole("button", { name: /Nurul Aisyah/ });
    expect(first.getAttribute("aria-current")).toBe("true");
    expect(first.textContent).toContain("RM 8,000 → RM 9,500 · 1 month");
    expect(first.textContent).toContain("in CV");
  });

  it("moves with Next, Prev, the arrow keys and a click, and marks those read", async () => {
    serveCvs();
    const { seen } = reader();
    await screen.findByText("the CV of cand_1");
    fireEvent.click(screen.getByRole("button", { name: "Next ›" }));
    expect(await screen.findByText("the CV of cand_2")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("2 of 3");
    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(await screen.findByText(/No CV on file for this candidate yet/)).toBeTruthy(); // Siti has none
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Next ›" }).disabled).toBe(true);
    fireEvent.keyDown(window, { key: "ArrowLeft" });
    expect(await screen.findByText("the CV of cand_2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "‹ Prev" }));
    expect(await screen.findByText("the CV of cand_1")).toBeTruthy();
    expect([...seen]).toEqual(["cand_1", "cand_2", "cand_3"]);
    fireEvent.click(screen.getByRole("button", { name: /Siti Hajar/ }));
    expect(screen.getByRole("region", { name: "Resume of Siti Hajar" })).toBeTruthy();
  });

  it("does not move while the person types", async () => {
    serveCvs();
    reader({ canAddToJob: true });
    await screen.findByText("the CV of cand_1");
    fireEvent.click(screen.getByRole("button", { name: "+ Add to job" }));
    fireEvent.keyDown(screen.getByPlaceholderText("Type a job title"), { key: "ArrowRight" });
    expect(screen.getByRole("status").textContent).toBe("1 of 3");
  });

  it("keeps pay hidden from those who may not see it", async () => {
    serveCvs();
    reader();
    fireEvent.click(screen.getByRole("button", { name: /Wei Tan/ }));
    const pane = await screen.findByRole("region", { name: "Resume of Wei Tan" });
    expect(pane.textContent).toContain("Restricted");
    expect(pane.textContent).not.toContain("RM");
    expect(screen.getByRole("button", { name: /Wei Tan/ }).textContent).toContain("1 month");
  });

  it("chooses the candidate to email", async () => {
    serveCvs();
    const onChoose = vi.fn();
    reader({ chosen: new Set(["cand_2"]), onChoose });
    fireEvent.click(await screen.findByRole("button", { name: "Choose to email" }));
    expect(onChoose).toHaveBeenCalledWith(nurul, true);
    expect(screen.getByRole("button", { name: /Wei Tan/ }).textContent).toContain("chosen");
  });

  it("adds the candidate to an open job", async () => {
    serveCvs();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "app_1" }));
    reader({ canAddToJob: true });
    fireEvent.click(await screen.findByRole("button", { name: "+ Add to job" }));
    fireEvent.change(screen.getByPlaceholderText("Type a job title"), { target: { value: "finance" } });
    fireEvent.click(await screen.findByRole("button", { name: /Finance Manager/ }));
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith("/v1/jobs/{jobId}/applications", { params: { path: { jobId: "job_1" } }, body: { candidateId: "cand_1" } });
    });
    expect(await screen.findByText("✓ Added to Finance Manager")).toBeTruthy();
  });

  it("has no Add to job without the permission", async () => {
    serveCvs();
    reader();
    await screen.findByText("the CV of cand_1");
    expect(screen.queryByRole("button", { name: "+ Add to job" })).toBeNull();
  });
});
