import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { FormatCv } from "./FormatCv";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const reads = () =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/cv-templates"
    ? { items: [{ id: "standard", name: "HiringPlug standard", builtIn: true, uploadedByName: null, createdAt: null }, { id: "ctpl_1", name: "Branded", builtIn: false, uploadedByName: "Aina", createdAt: "2026-11-01T00:00:00Z" }], placeholders: [], blocks: [] }
    : { items: [{ id: "appl_1", jobTitle: "Finance Manager", companyName: "Contoh" }] }))) as never);

describe("formatted CVs", () => {
  it("makes one for a job, contact hidden unless asked", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ documentId: "doc_1", fileName: "Siti - CV.docx" }));
    const onDone = vi.fn();
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create", "pipeline:read"]} onDone={onDone} />);
    fireEvent.change(await screen.findByLabelText("For"), { target: { value: "appl_1" } });
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "ctpl_1" } });
    expect(screen.queryByLabelText(/Include current and expected salary/)).toBeNull(); // only with salary permission
    fireEvent.click(screen.getByRole("button", { name: "Make formatted CV" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ templateId: "ctpl_1", showContact: false, includeSalary: false, applicationId: "appl_1" });
  });

  it("offers salary to those who may see it", async () => {
    reads();
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create", "candidates.compensation:read"]} onDone={vi.fn()} />);
    expect(await screen.findByLabelText(/Include current and expected salary/)).toBeTruthy();
  });
});
