import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { ImportPage } from "./ImportPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const item = (over: Partial<Schemas["CvImportItem"]>): Schemas["CvImportItem"] => ({
  id: "cvf_01jbz6w0m0k8q9x2r3s4t5v6w7", fileName: "Siti Aisyah.pdf", status: "created",
  candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", candidateName: "Siti Aisyah", error: null, updatedAt: "2026-11-05T02:00:00Z", ...over,
});
const anImport = (items: Schemas["CvImportItem"][]): Schemas["CvImport"] => {
  const counts = { reading: 0, created: 0, added: 0, unread: 0, failed: 0 };
  for (const i of items) counts[i.status]++;
  return { id: "cvi_01jbz6w0m0k8q9x2r3s4t5v6w7", source: "linkedin", createdAt: "2026-11-05T02:00:00Z", counts, items };
};
const reads = (imports: Schemas["CvImport"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(ok(path === "/v1/settings/fields" ? testFieldSettings() : { items: imports }))) as never);

describe("importing CVs and LinkedIn PDFs", () => {
  it("is offered only with the permission; the spreadsheet import keeps its own", () => {
    reads([]);
    renderWithClient(<ImportPage permissions={["intake:create"]} />);
    expect(screen.getByRole("region", { name: "CV import" })).toBeTruthy();
    expect(screen.queryByRole("region", { name: "CSV import" })).toBeNull();
    expect(screen.queryByText("Your role cannot add records.")).toBeNull();
    cleanup();
    vi.restoreAllMocks();

    const get = reads([]);
    renderWithClient(<ImportPage permissions={["companies:create"]} />);
    expect(screen.queryByRole("region", { name: "CV import" })).toBeNull();
    expect(screen.getByRole("region", { name: "CSV import" })).toBeTruthy();
    expect(get).not.toHaveBeenCalledWith("/v1/cv-imports");
    cleanup();

    renderWithClient(<ImportPage permissions={[]} />);
    expect(screen.getByText("Your role cannot add records.")).toBeTruthy();
  });

  it("starts an import with the chosen source and sends each file, saying which ones failed", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(anImport([])));
    const send = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(item({ status: "reading" })), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        error: { code: "PAYLOAD_TOO_LARGE", message: "The file is too large.", requestId: "req_1" },
      }), { status: 413 }));
    vi.stubGlobal("fetch", send);
    renderWithClient(<ImportPage permissions={["intake:create"]} />);

    const picker = screen.getByRole("group", { name: "Source of these candidates" });
    expect((await within(picker).findByRole("button", { name: /LinkedIn/ })).getAttribute("aria-pressed")).toBe("true");
    fireEvent.change(screen.getByLabelText("CV files"), {
      target: { files: [new File(["%PDF"], "Siti Aisyah.pdf"), new File(["%PDF"], "huge.pdf")] },
    });
    fireEvent.click(screen.getByRole("button", { name: "Import 2 files" }));

    expect(await screen.findByText("huge.pdf: The file is too large.")).toBeTruthy();
    expect(post).toHaveBeenCalledWith("/v1/cv-imports", expect.anything());
    expect(sentBody(post)).toEqual({ source: "linkedin" });
    expect(send).toHaveBeenCalledTimes(2);
    const [url, init] = send.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/cv-imports/cvi_01jbz6w0m0k8q9x2r3s4t5v6w7/files");
    expect(((init.body as FormData).get("file") as File).name).toBe("Siti Aisyah.pdf");
    expect(new Headers(init.headers).has("Authorization")).toBe(true);
    // The picker is emptied, so the same files can be picked again.
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("CV files").files?.length ?? 0).toBe(0); });
    expect(screen.getByRole("button", { name: "Import 0 files" })).toBeTruthy();
  });

  it("shows each file's result, with a link to its candidate", async () => {
    reads([anImport([
      item({}),
      item({ id: "cvf_2", fileName: "ahmad.pdf", status: "added", candidateName: "Ahmad Faiz", candidateId: "cand_2" }),
      item({ id: "cvf_3", fileName: "scan.pdf", status: "unread", candidateName: "scan", candidateId: "cand_3" }),
      item({ id: "cvf_4", fileName: "photo.png", status: "failed", candidateId: null, candidateName: null, error: "not a PDF, .docx or .doc file" }),
      item({ id: "cvf_5", fileName: "new.pdf", status: "reading", candidateName: "new", candidateId: "cand_5" }),
    ])]);
    renderWithClient(<ImportPage permissions={["intake:create"]} />);
    expect(await screen.findByText(/1 reading · 1 new · 1 added to existing · 1 to fill in · 1 failed/)).toBeTruthy();
    const row = (file: string) => screen.getByText(file).closest("tr") as HTMLElement;
    expect(within(row("Siti Aisyah.pdf")).getByRole("link", { name: "Siti Aisyah" }).getAttribute("href"))
      .toBe("#/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7");
    expect(row("ahmad.pdf").textContent).toContain("Added to existing candidate");
    expect(row("scan.pdf").textContent).toContain("fill in the details");
    expect(row("photo.png").textContent).toContain("not a PDF, .docx or .doc file");
    expect(within(row("photo.png")).queryByRole("link")).toBeNull();
    await waitFor(() => { expect(row("new.pdf").textContent).toContain("Reading…"); });
  });
});
