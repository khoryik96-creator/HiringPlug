import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company, type ContactSummary } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { CompanyContacts } from "./CompanyContacts";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const company = { id: "comp_1", name: "Maju Sdn Bhd" } as Company;
const contact = (fullName: string, email: string | null, mailboxStatus: ContactSummary["mailboxStatus"]): ContactSummary => ({
  id: `cont_${fullName}`, fullName, title: null, companyId: "comp_1", companyName: "Maju Sdn Bhd", email, emailCheckStatus: email ? "valid" : null,
  mailboxStatus, linkedinEmployment: null, phone: null, mobile: null, status: "active", role: null, updatedAt: "2026-11-01T00:00:00Z",
});

describe("a company's contacts", () => {
  it("verify every email at once, and the ones that bounce are marked in red", async () => {
    const before = [contact("Ana", "ana@maju.test", null), contact("Gone", "gone@maju.test", null), contact("Nobody", null, null)];
    const after = [contact("Ana", "ana@maju.test", "accepted"), contact("Gone", "gone@maju.test", "invalid"), contact("Nobody", null, null)];
    let verified = false;
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/contacts" ? { items: verified ? after : before, nextCursor: null } : testFieldSettings(),
    ))) as never);
    const post = vi.spyOn(api, "POST").mockImplementation((() => { verified = true; return Promise.resolve(ok({ started: 2, skipped: 0 })); }) as never);
    renderWithClient(<CompanyContacts company={company} canCreate={false} canCheck />);
    fireEvent.click(await screen.findByRole("button", { name: "Verify emails" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/contacts/mailbox-checks");
    expect(sentBody(post)).toEqual({ companyId: "comp_1" });

    expect((await screen.findByRole("status")).textContent).toContain("1 contact's email bounces");
    const gone = screen.getByRole("link", { name: "Gone" }).closest("tr");
    expect(gone?.className).toContain("bounces");
    expect(within(gone as HTMLElement).getByText("✗ Bounces")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Ana" }).closest("tr")?.className ?? "").not.toContain("bounces");
    expect(screen.getByText("✓ Mailbox exists")).toBeTruthy();
  });

  it("cannot be verified without contacts:update", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/contacts" ? { items: [contact("Ana", "ana@maju.test", null)], nextCursor: null } : testFieldSettings(),
    ))) as never);
    renderWithClient(<CompanyContacts company={company} canCreate={false} />);
    await screen.findByRole("link", { name: "Ana" });
    expect(screen.queryByRole("button", { name: "Verify emails" })).toBeNull();
  });
});
