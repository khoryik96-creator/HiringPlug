import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { JobsNeedingContact, LeaverMoves } from "./FollowLeaver";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const move = { companyUrl: "https://www.linkedin.com/company/acme-bhd/", companyName: "Acme Bhd", title: "Head of HR", since: "2025-09" };
const ana = {
  id: "cont_ana", firstName: "Ana", lastName: "Lim", fullName: "Ana Lim", title: "HR Manager", companyName: "Maju Sdn Bhd",
  linkedinUrl: "https://www.linkedin.com/in/ana-lim-example", role: "hr",
};

describe("following a contact who left", () => {
  it("adds them at their new company, which is new, with a lead note on it", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/companies" ? { items: [], nextCursor: null } : ana))) as never);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/companies" ? { id: "comp_acme", name: "Acme Bhd" } : path === "/v1/contacts" ? { id: "cont_new", fullName: "Ana Lim" } : { id: "note_1" },
    ))) as never);
    renderWithClient(<LeaverMoves contactId="cont_ana" moves={[move]} />);
    expect(screen.getByText(/Now Head of HR at/).textContent).toContain("since Sep 2025");
    fireEvent.click(screen.getByRole("button", { name: "Add at Acme Bhd" }));
    expect((await screen.findByRole("status")).textContent).toContain("Added: Ana Lim at Acme Bhd (new company, with a lead note)");
    const sent = Object.fromEntries(post.mock.calls.map((c) => [c[0], (c[1] as { body: unknown }).body]));
    expect(sent["/v1/companies"]).toEqual({ name: "Acme Bhd", linkedinUrl: move.companyUrl });
    expect(sent["/v1/contacts"]).toEqual({
      companyId: "comp_acme", firstName: "Ana", lastName: "Lim", title: "Head of HR", linkedinUrl: ana.linkedinUrl, role: "hr",
      notes: "Moved from Maju Sdn Bhd in Sep 2025 (seen on LinkedIn).",
    });
    expect(sent["/v1/notes"]).toEqual({
      subjectType: "company", subjectId: "comp_acme",
      body: "Lead: Ana Lim, HR Manager at Maju Sdn Bhd (our contact), joined Acme Bhd in Sep 2025 as Head of HR.",
    });
  });

  it("uses the company already here with that LinkedIn page", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/companies" ? { items: [{ id: "comp_acme", name: "Acme Berhad" }], nextCursor: null }
        : path === "/v1/contacts" ? { items: [{ id: "cont_other", fullName: "Ana Tan" }], nextCursor: null } : ana,
    ))) as never);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/contacts" ? { id: "cont_new", fullName: "Ana Lim" } : {}))) as never);
    renderWithClient(<LeaverMoves contactId="cont_ana" moves={[move]} />);
    fireEvent.click(screen.getByRole("button", { name: "Add at Acme Bhd" }));
    expect((await screen.findByRole("status")).textContent).toContain("at Acme Berhad (with a lead note)");
    const lookup = (get.mock.calls as unknown[][]).find((c) => c[0] === "/v1/companies");
    expect(lookup?.[1]).toEqual({ params: { query: { linkedinCompany: "acme-bhd", limit: 1 } } });
    expect(post.mock.calls.map((c) => c[0])).not.toContain("/v1/companies");
  });

  it("does not add them twice where they are a contact already", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/companies" ? { items: [{ id: "comp_acme", name: "Acme Berhad" }], nextCursor: null }
        : path === "/v1/contacts" ? { items: [{ id: "cont_followed", fullName: "ana lim" }], nextCursor: null } : ana,
    ))) as never);
    const post = vi.spyOn(api, "POST");
    renderWithClient(<LeaverMoves contactId="cont_ana" moves={[move]} />);
    fireEvent.click(screen.getByRole("button", { name: "Add at Acme Bhd" }));
    expect((await screen.findByRole("status")).textContent).toBe("Already a contact: ana lim at Acme Berhad");
    expect(screen.getByRole("link", { name: "ana lim" }).getAttribute("href")).toBe("#/contacts/cont_followed");
    expect((get.mock.calls as unknown[][]).find((c) => c[0] === "/v1/contacts")?.[1]).toEqual({ params: { query: { companyId: "comp_acme", q: "Ana Lim", limit: 20 } } });
    expect(post).not.toHaveBeenCalled();
  });

  it("lists the open jobs they were the hiring contact for", async () => {
    vi.spyOn(api, "GET").mockImplementation(((_: string, init: { params: { query: { status: string } } }) => Promise.resolve(ok({
      items: init.params.query.status === "open" ? [{ id: "jo_1", title: "Finance Manager" }] : [{ id: "jo_2", title: "Auditor" }], nextCursor: null,
    }))) as never);
    renderWithClient(<JobsNeedingContact contactId="cont_ana" />);
    await waitFor(() => { expect(screen.getByRole("status").textContent).toContain("Hiring contact on 2 open jobs"); });
    expect(screen.getByRole("link", { name: "Auditor" }).getAttribute("href")).toBe("#/jobs/jo_2");
  });
});
