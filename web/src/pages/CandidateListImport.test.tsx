import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const summary = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Siti Aisyah", email: null, phone: null, currentTitle: "Accountant",
  currentCompany: null, location: null, workAuthorizationStatus: null, tags: [], updatedAt: "2026-11-01T00:00:00Z",
  availability: null, availabilitySince: null, noticePeriod: null, currentSalary: null, expectedSalary: null, salaryRestricted: false,
  workAuthorizationBasis: null, nationality: null, residenceCountry: null, source: null, industry: null,
};
const pdf = (name: string) => new File(["%PDF-1.7"], name, { type: "application/pdf" });
const dropOn = (el: Element, files: File[]) => {
  fireEvent.dragEnter(el, { dataTransfer: { types: ["Files"], files } });
  fireEvent.drop(el, { dataTransfer: { types: ["Files"], files } });
};

describe("CVs dropped while an import is open", () => {
  it("join the open import, wherever on the page they are dropped", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
      path === "/v1/candidates" ? ok({ items: [summary], nextCursor: null })
        : path === "/v1/settings/fields" ? ok(testFieldSettings())
          : path === "/v1/me/preferences" ? ok({ candidateColumns: null })
            : ok({ items: [], tags: [], skills: [], industries: [], specializations: [], certifications: [] }),
    )) as never);
    renderWithClient(<CandidateList canImport />);
    dropOn(await screen.findByText("Siti Aisyah"), [pdf("first-cv.pdf")]);
    const panel = await screen.findByRole("region", { name: "CV import" });
    expect(screen.queryByText(/Drop CVs anywhere to upload/), "no second way in while one is open").toBeNull();
    dropOn(screen.getByText("Siti Aisyah"), [pdf("second-cv.pdf")]);
    dropOn(screen.getByText("Siti Aisyah"), [pdf("third-cv.pdf"), pdf("first-cv.pdf")]);
    expect(screen.getByRole("region", { name: "CV import" }), "the same import").toBe(panel);
    expect(panel.textContent).toContain("first-cv.pdf");
    expect(panel.textContent, "the second drop joins it").toContain("second-cv.pdf");
    expect(panel.textContent, "and the third").toContain("third-cv.pdf");
    expect(panel.textContent.split("first-cv.pdf").length, "a CV dropped again is not added twice").toBe(2);
  });
});
