import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { CvImports } from "../components/CvImports";
import { Dashboard } from "./Dashboard";
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
  const counts = { reading: 0, created: 0, added: 0, unread: 0, failed: 0, duplicate: 0 };
  for (const i of items) counts[i.status]++;
  return { id: "cvi_01jbz6w0m0k8q9x2r3s4t5v6w7", source: "linkedin", createdAt: "2026-11-05T02:00:00Z", counts, items };
};
const reads = (imports: Schemas["CvImport"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(ok(path === "/v1/settings/fields" ? testFieldSettings() : { items: imports }))) as never);

describe("importing CVs and LinkedIn PDFs", () => {
  it("is on the dashboard, only with the permission; the Import page keeps the spreadsheet import", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/pipeline/summary"
      ? { openJobs: 0, stageCounts: { applied: 0, screening: 0, submitted: 0, interview: 0, offer: 0, placed: 0 }, placedLast30Days: 0, movedLast7Days: 0, jobs: [], stuck: [] }
      : { items: [] }))) as never);
    renderWithClient(<Dashboard permissions={["intake:create", "pipeline:read"]} />);
    expect(await screen.findByRole("region", { name: "CV import" })).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Recent moves" })).toBeNull();
    cleanup();

    renderWithClient(<Dashboard permissions={["pipeline:read"]} />);
    await screen.findByRole("region", { name: "Open jobs" });
    expect(screen.queryByRole("region", { name: "CV import" })).toBeNull();
    cleanup();
    vi.restoreAllMocks();

    reads([]);
    renderWithClient(<ImportPage permissions={["intake:create"]} />);
    expect(screen.queryByRole("region", { name: "CV import" })).toBeNull();
    expect(screen.getByRole("link", { name: "dashboard" })).toBeTruthy();
    cleanup();

    const get = reads([]);
    renderWithClient(<ImportPage permissions={["companies:create"]} />);
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
    renderWithClient(<CvImports />);

    const picker = screen.getByRole("group", { name: "Source of these candidates" });
    expect((await within(picker).findByRole("button", { name: /LinkedIn/ })).getAttribute("aria-pressed")).toBe("true");
    fireEvent.change(screen.getByLabelText("Drop CVs here"), {
      target: { files: [new File(["%PDF"], "Siti Aisyah.pdf"), new File(["%PDF"], "huge.pdf")] },
    });
    fireEvent.click(screen.getByRole("button", { name: "Import 2 files" }));

    expect(await screen.findByText("huge.pdf: The file is too large.")).toBeTruthy();
    expect(post).toHaveBeenCalledWith("/v1/cv-imports", { body: { source: "linkedin" } });
    expect(post).toHaveBeenCalledWith("/v1/document-matches", expect.anything()); // looked for CVs already here first
    expect(send).toHaveBeenCalledTimes(2);
    const [url, init] = send.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/cv-imports/cvi_01jbz6w0m0k8q9x2r3s4t5v6w7/files");
    expect(((init.body as FormData).get("file") as File).name).toBe("Siti Aisyah.pdf");
    expect(new Headers(init.headers).has("Authorization")).toBe(true);
    // The list is emptied once sent.
    await waitFor(() => { expect(screen.getByRole("button", { name: "Import 0 files" })).toBeTruthy(); });
    expect(screen.queryByRole("list", { name: "Files to import" })).toBeNull();
  });

  it("takes CVs dropped on it, in several drops, skipping other files and the same file twice", async () => {
    reads([]);
    renderWithClient(<CvImports />);
    const zone = (await screen.findByText("Drop CVs here")).closest(".drop-zone");
    if (!zone) throw new Error("no drop zone");
    const siti = new File(["%PDF"], "Siti.pdf");
    fireEvent.drop(zone, { dataTransfer: { files: [siti, new File(["x"], "slides.pptx")] } });
    expect(screen.getByText(/Skipped .*: slides.pptx/)).toBeTruthy();
    fireEvent.drop(zone, { dataTransfer: { files: [siti, new File(["PK"], "Ahmad.docx")] } });
    const list = screen.getByRole("list", { name: "Files to import" });
    expect(within(list).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Siti.pdf×", "Ahmad.docx×"]);
    fireEvent.click(screen.getByRole("button", { name: "Remove Siti.pdf" }));
    expect(screen.getByRole("button", { name: "Import 1 file" })).toBeTruthy();
  });

  it("shows each file's result, with a link to its candidate", async () => {
    reads([anImport([
      item({}),
      item({ id: "cvf_2", fileName: "ahmad.pdf", status: "added", candidateName: "Ahmad Faiz", candidateId: "cand_2" }),
      item({ id: "cvf_3", fileName: "scan.pdf", status: "unread", candidateName: "scan", candidateId: "cand_3" }),
      item({ id: "cvf_4", fileName: "photo.png", status: "failed", candidateId: null, candidateName: null, error: "not a PDF, .docx or .doc file" }),
      item({ id: "cvf_5", fileName: "new.pdf", status: "reading", candidateName: "new", candidateId: "cand_5" }),
    ])]);
    renderWithClient(<CvImports />);
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

  it("skips files already in HiringPlug and asks about ones with the same name", async () => {
    reads([]);
    const match = (fileName: string) => ({ documentId: "doc_1", candidateId: "cand_9", candidateName: "Nurul Huda", fileName, createdAt: "2026-11-03T02:00:00Z" });
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/document-matches"
      ? { items: [
        { fileName: "same.pdf", sha256: "a", sameContent: match("Nurul CV.pdf"), sameName: [] },
        { fileName: "Nurul CV.pdf", sha256: "b", sameContent: null, sameName: [match("Nurul CV.pdf")] },
        { fileName: "new.pdf", sha256: "c", sameContent: null, sameName: [] },
      ] }
      : anImport([])))) as never);
    const send = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify(item({ status: "reading" })), { status: 201 })));
    vi.stubGlobal("fetch", send);
    renderWithClient(<CvImports />);
    await within(screen.getByRole("group", { name: "Source of these candidates" })).findByRole("button", { name: /LinkedIn/ });
    fireEvent.change(screen.getByLabelText("Drop CVs here"), {
      target: { files: [new File(["one"], "same.pdf"), new File(["two"], "Nurul CV.pdf"), new File(["three"], "new.pdf")] },
    });
    fireEvent.click(screen.getByRole("button", { name: "Import 3 files" }));

    const skipped = await screen.findByRole("status", { name: "Skipped files" });
    expect(skipped.textContent).toContain("same.pdf: Nurul Huda's CV (Nurul CV.pdf, added 3 Nov 2026)");
    const ask = screen.getByRole("group", { name: "Files with the same name as a CV already here" });
    expect(send).not.toHaveBeenCalled(); // waiting for the answer
    fireEvent.click(screen.getByRole("button", { name: "Import 1 file" })); // only new.pdf: Nurul CV.pdf not ticked
    await waitFor(() => { expect(send).toHaveBeenCalledTimes(1); });
    expect(((send.mock.calls[0] as [string, RequestInit])[1].body as FormData).get("file")).toHaveProperty("name", "new.pdf");
    expect(ask).toBeTruthy();
    expect(post).toHaveBeenCalledWith("/v1/cv-imports", expect.anything());
  });
});
