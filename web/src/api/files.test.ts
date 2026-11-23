import { afterEach, describe, expect, it, vi } from "vitest";

import { downloadExport } from "./files";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("downloadExport", () => {
  it("sends the search and saves the file under the name the API gives", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response("x", {
      status: 200, headers: { "Content-Disposition": `attachment; filename="Candidates 22 Nov 2026.xlsx"`, "Content-Type": "application/octet-stream" },
    }));
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("URL", class extends URL { static override createObjectURL = vi.fn().mockReturnValue("blob:x"); static override revokeObjectURL = vi.fn(); });
    let saved = "";
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) { saved = this.download; });
    await downloadExport("/v1/candidates/export", { format: "xlsx", search: { q: "Siti" } });
    expect(saved).toBe("Candidates 22 Nov 2026.xlsx");
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/v1/candidates/export");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ format: "xlsx", search: { q: "Siti" } });
  });

  it("throws the API's error (too many rows)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "EXPORT_TOO_LARGE", message: "That finds more than 10000. Narrow the search and export again.", requestId: "req_1" },
    }), { status: 422 })));
    await expect(downloadExport("/v1/companies/export?format=csv")).rejects.toThrow(/Narrow the search/);
  });
});
