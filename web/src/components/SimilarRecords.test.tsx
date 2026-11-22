import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { SimilarCompanies, SimilarContacts } from "./SimilarRecords";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const company = { id: "comp_keep", name: "Acme Sdn Bhd", version: 3 };
const lookalike = { id: "comp_dup", name: "ACME Berhad", city: "Penang", website: "acme.example", industry: null, reasons: ["same_website", "similar_name"] };

describe("SimilarCompanies", () => {
  it("shows nothing without possible duplicates", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<SimilarCompanies company={company} canMerge />);
    await waitFor(() => { expect(get).toHaveBeenCalled(); });
    expect(screen.queryByRole("region", { name: "Possible duplicates" })).toBeNull();
  });

  it("merges a duplicate into this company after asking", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [lookalike] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...company, version: 4 }));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<SimilarCompanies company={company} canMerge />);
    expect(await screen.findByText("1 possible duplicate")).toBeTruthy();
    expect(screen.getByText(/same website or similar name/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Review and merge" }));
    fireEvent.click(screen.getByRole("button", { name: "Merge into this one" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/companies/{companyId}/merge");
    expect(sentBody(post)).toEqual({ duplicateId: "comp_dup", version: 3 });
  });

  it("only lists them for those who may not merge", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [lookalike] }));
    renderWithClient(<SimilarCompanies company={company} canMerge={false} />);
    fireEvent.click(await screen.findByRole("button", { name: "Review" }));
    expect(screen.queryByRole("button", { name: "Merge into this one" })).toBeNull();
  });
});

describe("SimilarContacts", () => {
  it("does not offer to merge someone at another company", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      { id: "cont_here", companyId: "comp_a", companyName: "Acme", fullName: "Rahman Aisyah", title: null, email: null, reasons: ["same_name"] },
      { id: "cont_there", companyId: "comp_b", companyName: "Other", fullName: "Aisyah Rahman", title: null, email: null, reasons: ["same_name"] },
    ] }));
    renderWithClient(<SimilarContacts contact={{ id: "cont_keep", companyId: "comp_a", firstName: "Aisyah", lastName: "Rahman", version: 1 }} canMerge />);
    fireEvent.click(await screen.findByRole("button", { name: "Review and merge" }));
    expect(screen.getAllByRole("button", { name: "Merge into this one" })).toHaveLength(1);
    expect(screen.getByText(/at another company/)).toBeTruthy();
  });
});
