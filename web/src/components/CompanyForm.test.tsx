import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { CompanyForm } from "./CompanyForm";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const company: Company = {
  id: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 2, name: "Maybank", type: "client", category: null,
  website: "https://maybank.com", industry: "Banking", size: "enterprise", phone: null, address: null,
  city: "Kuala Lumpur", country: "MY", registrationNumber: null, about: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

const change = (label: string, value: string) => {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
};

describe("company form", () => {
  it("adds a company with every field and hands it back", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(company));
    const onSaved = vi.fn();
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} onSaved={onSaved} />);
    change("Name", " Maybank ");
    change("Website", "maybank.com");
    change("Size", "enterprise");
    change("Country (2-letter code)", "my");
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    await waitFor(() => { expect(onSaved).toHaveBeenCalledWith(company); });
    expect(sentBody(post)).toMatchObject({ name: "Maybank", type: "client", website: "maybank.com", size: "enterprise", country: "MY", city: null });
  });

  it("asks for a name before sending", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(company));
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    expect(await screen.findByText("Enter the company's name.")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });

  it("says when the name is taken", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(failure("COMPANY_DUPLICATE_NAME", "Another company already has this name.", { existingCompanyId: company.id }, 409));
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("Name", "Maybank");
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Another company already has this name.");
  });

  it("saves an edit with the version it was opened with", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...company, version: 4 }));
    const onDone = vi.fn();
    const { rerender } = renderWithClient(<CompanyForm company={company} onDone={onDone} />);
    rerender(<CompanyForm company={{ ...company, version: 3, phone: "+60 3-2070 8833" }} onDone={onDone} />);
    change("City", "Petaling Jaya");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ version: 2, city: "Petaling Jaya" });
  });

  it("closes without saving when nothing changed", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(company));
    const onDone = vi.fn();
    renderWithClient(<CompanyForm company={company} onDone={onDone} />);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(patch).not.toHaveBeenCalled();
  });
});
