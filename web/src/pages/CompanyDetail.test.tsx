import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company } from "../api/client";
import { ok, RECRUITER, renderWithClient } from "../test-utils";
import { CompanyDetail } from "./CompanyDetail";
import { CompanyList } from "./CompanyList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const company: Company = {
  id: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, name: "Grab", type: "prospect",
  website: "https://grab.com", industry: "Technology", size: "large", phone: null, address: "1 Fusionopolis\nSingapore",
  city: "Singapore", country: "SG", registrationNumber: "201221800W", about: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

describe("company pages", () => {
  it("shows a company in words, with an edit button for those who may edit", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(company));
    renderWithClient(<CompanyDetail id={company.id} permissions={RECRUITER.concat("companies:update")} />);
    await screen.findByRole("heading", { name: "Grab" });
    expect(screen.getByText("Prospect · Technology · Singapore, Singapore (SG)")).toBeTruthy();
    expect(screen.getByText("201–1,000 people")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit company" })).toBeTruthy();
  });

  it("gives viewers no edit or add buttons", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(company));
    renderWithClient(<CompanyDetail id={company.id} permissions={["companies:read"]} />);
    await screen.findByRole("heading", { name: "Grab" });
    expect(screen.queryByRole("button", { name: "Edit company" })).toBeNull();
    cleanup();

    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [company], nextCursor: null }));
    renderWithClient(<CompanyList canCreate={false} />);
    await screen.findByRole("link", { name: "Grab" });
    expect(screen.queryByRole("button", { name: "Add company" })).toBeNull();
  });
});
