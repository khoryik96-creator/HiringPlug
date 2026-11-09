import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { StudioCvRuns } from "./StudioCvRuns";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const run = (over: Partial<Schemas["StudioCv"]>): Schemas["StudioCv"] => ({
  id: "scv_1", candidateId: "cand_1", sourceDocumentId: "doc_o", status: "queued", blind: false, correctLanguage: true,
  alignment: "left", documentId: null, fileName: null, error: null, warning: null, payStatementsRemoved: 0,
  createdAt: new Date().toISOString(), ...over,
});

const serve = (items: Schemas["StudioCv"][]) => vi.spyOn(api, "GET").mockResolvedValue(ok({ items }));

describe("CV Studio formatting runs", () => {
  it("says one is being made", async () => {
    serve([run({ blind: true })]);
    renderWithClient(<StudioCvRuns candidateId="cand_1" />);
    expect((await screen.findByRole("status")).textContent).toContain("(blind)");
  });

  it("asks to compare a saved CV with the original when CV Studio warns", async () => {
    serve([run({ status: "done", documentId: "doc_2", fileName: "Siti - CV.docx", warning: "a role may be missing: Audit Associate", payStatementsRemoved: 1 })]);
    renderWithClient(<StudioCvRuns candidateId="cand_1" />);
    expect(await screen.findByText("Siti - CV.docx")).toBeTruthy();
    expect(screen.getByText(/Compare it with the CV before sending: a role may be missing/)).toBeTruthy();
    expect(screen.getByText("(salary left out)")).toBeTruthy();
  });

  it("says why one failed, and leaves out old ones", async () => {
    serve([run({ status: "failed", error: "It could not be formatted: no AI provider is set up" }),
      run({ id: "scv_0", status: "failed", error: "old", createdAt: "2026-01-01T00:00:00Z" })]);
    renderWithClient(<StudioCvRuns candidateId="cand_1" />);
    expect((await screen.findByRole("alert")).textContent).toContain("no AI provider is set up");
    expect(screen.queryByText("old")).toBeNull();
  });

  it("leaves out a run whose formatted CV was deleted", async () => {
    serve([run({ status: "done", documentId: "doc_gone", fileName: "Gone - CV.docx" }), run({ id: "scv_2", status: "done", documentId: "doc_2", fileName: "Kept - CV.docx" })]);
    renderWithClient(<StudioCvRuns candidateId="cand_1" documentIds={new Set(["doc_2"])} />);
    expect(await screen.findByText("Kept - CV.docx")).toBeTruthy();
    expect(screen.queryByText("Gone - CV.docx")).toBeNull();
  });
});
