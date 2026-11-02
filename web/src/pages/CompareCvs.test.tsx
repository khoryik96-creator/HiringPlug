import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Document, type Schemas } from "../api/client";
import { compareFields, compareJobs, compareSkills } from "../components/compareCvs";
import { ok, renderWithClient } from "../test-utils";
import { CompareCvs } from "./CompareCvs";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const s = (value: string, quote = value) => ({ value, quote });
const job = (title: string, company: string, startDate: string, endDate: string | null, current = false) => ({
  title: s(title), company: s(company), dates: null, startDate, endDate, current,
});
const profile = (over: Partial<Schemas["ParsedProfile"]> = {}): Schemas["ParsedProfile"] => ({
  parser: "rules@1", fullName: s("Siti Aisyah"), email: s("siti@example.com"), phone: s("+60 12-345 6789"),
  linkedinUrl: null, location: null, nationality: null, noticePeriod: null, skills: [s("SAP FICO"), s("Excel")],
  experience: [job("Senior Accountant", "Contoh Berhad", "2020-01", null, true), job("Accountant", "ABC Sdn Bhd", "2016-03", "2019-12")],
  education: [], dropped: 0, ...over,
});

describe("comparing CV details", () => {
  it("lists only real differences: spacing, case and phone punctuation do not count", () => {
    const rows = compareFields(profile(), profile({ fullName: s("SITI  AISYAH"), phone: s("+6012-345 6789"), email: s("siti.a@example.com") }));
    expect(rows.filter((r) => r.differs).map((r) => [r.label, r.a, r.b])).toEqual([["Email", "siti@example.com", "siti.a@example.com"]]);
  });

  it("matches jobs by employer and reports changed dates, and jobs or skills only one CV has", () => {
    const formatted = profile({
      experience: [job("Senior Accountant", "Contoh Bhd", "2020-02", null, true)],
      skills: [s("excel"), s("Power BI")],
    });
    expect(compareJobs(profile(), formatted)).toEqual({
      onlyA: ["Accountant at ABC Sdn Bhd (Mar 2016 – Dec 2019)"],
      onlyB: [],
      changed: [{ a: "Senior Accountant at Contoh Berhad (Jan 2020 – Present)", b: "Senior Accountant at Contoh Bhd (Feb 2020 – Present)" }],
    });
    expect(compareSkills(profile(), formatted)).toEqual({ onlyA: ["SAP FICO"], onlyB: ["Power BI"] });
  });
});

describe("compare page", () => {
  const doc = (id: string, source: Document["source"], contentType: string, parsedProfile: Schemas["ParsedProfile"]) => ({
    id, candidateId: "cand_1", source, fileName: `${source}.pdf`, contentType, sizeBytes: 10, sha256: "0".repeat(64),
    status: "extracted", pageCount: 1, extractor: "x", errorMessage: null, extractionWarnings: [], parseStatus: "parsed",
    extractedText: `text of ${source}`, parsedProfile, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  });
  const docs = [
    doc("doc_f", "formatted", "application/msword", profile({ experience: [job("Senior Accountant", "Contoh Berhad", "2020-01", null, true)] })),
    doc("doc_o", "original", "application/pdf", profile()),
  ];

  it("puts the original next to another version, shows PDFs as they are, and lists differences", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string, init: { params: { path: { documentId?: string } } }) => {
      if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: docs }));
      return Promise.resolve(ok(docs.find((d) => d.id === init.params.path.documentId)));
    }));
    const fetchFile = vi.fn().mockResolvedValue(new Response(new Uint8Array([37, 80, 68, 70])));
    vi.stubGlobal("fetch", fetchFile);
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: () => "blob:cv", revokeObjectURL: vi.fn() }));
    renderWithClient(<CompareCvs candidateId="cand_1" />);

    const differences = await screen.findByRole("region", { name: "Differences" });
    expect(differences.textContent).toContain("Only in the original CV: Accountant at ABC Sdn Bhd (Mar 2016 – Dec 2019)");
    const original = await screen.findByTitle("original.pdf");
    expect(original.getAttribute("src")).toBe("blob:cv");
    expect(fetchFile).toHaveBeenCalledWith("/v1/documents/doc_o/file", expect.anything());
    // The Word file is shown as its text.
    await waitFor(() => { expect(screen.getByText("text of formatted")).toBeTruthy(); });
    expect(screen.getAllByRole("combobox").map((c) => (c as HTMLSelectElement).value)).toEqual(["doc_o", "doc_f"]);
  });
});
