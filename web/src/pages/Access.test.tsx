import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { Owner } from "../components/Owner";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Access, scopeText } from "./Access";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const role = (over: Partial<Schemas["RoleAccess"]>): Schemas["RoleAccess"] => ({
  role: "recruiter", permissions: ["candidates:read", "candidates.compensation:read"], candidateScope: "all", jobCandidates: true,
  custom: false, editable: true, version: 0, ...over,
});
const settings: Schemas["AccessSettings"] = {
  roles: [role({ role: "owner", editable: false }), role({}), role({ role: "sourcer", candidateScope: "own", custom: true, version: 3, permissions: ["candidates:read"] })],
  permissions: [
    { permission: "candidates:read", module: "candidates", description: "View candidates.", sensitive: false, defaultRoles: ["owner", "recruiter", "sourcer"] },
    { permission: "candidates.compensation:read", module: "candidates", description: "See salary.", sensitive: true, defaultRoles: ["owner", "recruiter"] },
    { permission: "identity.roles:manage", module: "identity", description: "Choose what roles may do.", sensitive: false, defaultRoles: ["owner"] },
  ],
};

describe("roles and access", () => {
  it("says who sees which candidates, in words", () => {
    expect(scopeText({ candidateScope: "all", jobCandidates: true })).toBe("All candidates");
    expect(scopeText({ candidateScope: "own", jobCandidates: true })).toBe("Their own candidates, and those on their jobs");
    expect(scopeText({ candidateScope: "team", jobCandidates: true })).toBe("Their team's candidates, and those on their team's jobs");
    expect(scopeText({ candidateScope: "team", jobCandidates: false })).toBe("Their team's candidates");
  });

  it("keeps the owner fixed, and saves a role's permissions and scope with its version", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(settings));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(role({ version: 1, custom: true })));
    renderWithClient(<Access />);
    expect((await screen.findByRole("region", { name: "Owner" })).textContent).toContain("Owners always have every permission");
    const recruiter = screen.getByRole("region", { name: "Recruiter" });
    const save = within(recruiter).getByRole("button", { name: "Save recruiter" });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    expect(within(recruiter).getByLabelText(/Choose what roles may do/).hasAttribute("disabled")).toBe(true);

    fireEvent.click(within(recruiter).getByLabelText(/See salary/));
    fireEvent.click(within(recruiter).getByLabelText("Their own candidates"));
    fireEvent.click(save);
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ version: 0, permissions: ["candidates:read"], candidateScope: "own", jobCandidates: true });
  });

  it("gives a changed role its defaults back", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(settings));
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok(role({ role: "sourcer" })));
    renderWithClient(<Access />);
    const sourcer = await screen.findByRole("region", { name: "Sourcer" });
    expect(sourcer.textContent).toContain("Changed by you");
    expect(within(screen.getByRole("region", { name: "Recruiter" })).queryByRole("button", { name: "Back to the defaults" })).toBeNull();
    fireEvent.click(within(sourcer).getByRole("button", { name: "Back to the defaults" }));
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
  });
});

describe("owners of candidates and jobs", () => {
  it("are shown, and can be handed to another user or to nobody", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ id: "usr_1", name: "Aina", role: "recruiter", canInterview: true }, { id: "usr_2", name: "Ben", role: "sourcer", canInterview: false }] }));
    const save = vi.fn().mockResolvedValue(undefined);
    renderWithClient(<Owner what="candidate" ownerId="usr_1" ownerName="Aina" canChange save={save} />);
    const select = await screen.findByLabelText<HTMLSelectElement>("Owner of this candidate");
    await waitFor(() => { expect(select.options.length).toBe(3); });
    fireEvent.change(select, { target: { value: "usr_2" } });
    await waitFor(() => { expect(save).toHaveBeenCalledWith("usr_2", expect.anything()); });
    fireEvent.change(select, { target: { value: "" } });
    await waitFor(() => { expect(save).toHaveBeenLastCalledWith(null, expect.anything()); });
    cleanup();
    renderWithClient(<Owner what="job" ownerId={null} ownerName={null} canChange={false} save={save} />);
    expect(screen.getByText("Nobody")).toBeTruthy();
  });

  it("filter the candidate list to your own", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], nextCursor: null, tags: [], skills: [], industries: [], specializations: [] }));
    renderWithClient(<CandidateList canCreate={false} />);
    fireEvent.click(await screen.findByLabelText("Mine"));
    await waitFor(() => {
      const calls = get.mock.calls as unknown as [string, { params?: { query?: { owner?: string } } }][];
      expect(calls.some(([path, init]) => path === "/v1/candidates" && init.params?.query?.owner === "me")).toBe(true);
    });
  });
});
