import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { CompanyDocuments } from "./CompanyDocuments";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const doc = (id: string, section: Schemas["CompanyDocument"]["section"], fileName: string): Schemas["CompanyDocument"] => ({
  id, section, fileName, contentType: "application/pdf", sizeBytes: 245_000, createdAt: "2026-11-09T03:00:00Z", createdByName: "Aina",
});
const listed = [doc("cdoc_1", "terms", "Signed TOB 2026.pdf"), doc("cdoc_2", "benefits", "Benefits brochure.pdf")];

describe("documents kept with a client's terms or benefits", () => {
  it("lists the section's own, and drops files in", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: listed }));
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("{}", { status: 201 }));
    renderWithClient(<CompanyDocuments companyId="comp_1" section="terms" permissions={["companies:read", "companies:update"]} />);
    expect(await screen.findByRole("button", { name: "Signed TOB 2026.pdf" })).toBeTruthy();
    expect(screen.queryByText("Benefits brochure.pdf")).toBeNull();
    expect(screen.getByText(/239 KB · added .* by Aina/)).toBeTruthy();
    const del = screen.getByRole<HTMLButtonElement>("button", { name: "Delete Signed TOB 2026.pdf" });
    expect(del.disabled, "deleting is a permission of its own (admins)").toBe(true);
    expect(del.title).toContain("Only admins delete documents");

    const file = new File(["%PDF-1.4"], "Addendum.pdf", { type: "application/pdf" });
    fireEvent.change(screen.getByLabelText("Drop the signed terms here"), { target: { files: [file] } });
    await waitFor(() => { expect(fetch).toHaveBeenCalled(); });
    const [url, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/companies/comp_1/documents");
    const body = init.body as FormData;
    expect(body.get("section")).toBe("terms");
    expect((body.get("file") as File).name).toBe("Addendum.pdf");
  });

  it("are deleted only by those given companies.documents:delete", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: listed }));
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok(undefined));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<CompanyDocuments companyId="comp_1" section="benefits" permissions={["companies:read", "companies.documents:delete"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Delete Benefits brochure.pdf" }));
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
    expect(del.mock.calls[0]?.[1]).toEqual({ params: { path: { companyId: "comp_1", documentId: "cdoc_2" } } });
    expect(screen.queryByLabelText("Drop a benefits document here"), "adding needs companies:update").toBeNull();
  });
});
