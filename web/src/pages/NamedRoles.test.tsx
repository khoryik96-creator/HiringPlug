import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Access } from "./Access";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const catalog: Schemas["PermissionInfo"][] = [
  { permission: "candidates:read", module: "candidates", description: "View candidates.", sensitive: false, defaultRoles: ["owner", "recruiter"] },
  { permission: "candidates.compensation:read", module: "candidates", description: "See salary.", sensitive: true, defaultRoles: ["owner", "recruiter"] },
];
const junior: Schemas["NamedRole"] = {
  id: "role_01aaaaaaaaaaaaaaaaaaaaaaaa", name: "Junior recruiter", baseRole: "recruiter", permissions: ["candidates:read", "candidates.compensation:read"],
  candidateScope: "all", jobCandidates: true, members: 2, version: 1,
};

function serve(named: Schemas["NamedRole"][]) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/access/named-roles" ? { items: named } : path === "/v1/agency/microsoft-sign-in" ? null : { roles: [], permissions: catalog },
  ))) as never);
}

describe("roles and access › named roles", () => {
  it("adds a role built on another, for owners only", async () => {
    serve([]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(junior));
    renderWithClient(<Access owner />);
    const box = await screen.findByRole("region", { name: "Named roles" });
    fireEvent.change(within(box).getByLabelText("Name of the new role"), { target: { value: "Junior recruiter" } });
    fireEvent.change(within(box).getByLabelText("Built on"), { target: { value: "sourcer" } });
    fireEvent.click(within(box).getByRole("button", { name: "Add role" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ name: "Junior recruiter", baseRole: "sourcer" });
    cleanup();
    serve([]);
    renderWithClient(<Access />);
    await screen.findByRole("heading", { name: "Roles and access" });
    expect(screen.queryByRole("region", { name: "Named roles" })).toBeNull();
  });

  it("renames a role and changes its access, saved with its version", async () => {
    serve([junior]);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...junior, version: 2 }));
    renderWithClient(<Access owner />);
    const box = await screen.findByRole("region", { name: "Named roles" });
    expect(await within(box).findByText(/built on Recruiter/)).toBeTruthy();
    expect(within(box).getByText(/2 people/)).toBeTruthy();
    fireEvent.change(within(box).getByLabelText("Name"), { target: { value: "Associate recruiter" } });
    fireEvent.click(within(box).getByLabelText(/See salary/));
    fireEvent.click(within(box).getByRole("button", { name: "Save Junior recruiter" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ name: "Associate recruiter", version: 1, permissions: ["candidates:read"], candidateScope: "all", jobCandidates: true });
  });
});
