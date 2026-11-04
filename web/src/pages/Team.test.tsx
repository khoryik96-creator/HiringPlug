import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Team } from "./Team";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const member = (id: string, name: string, role: Schemas["Role"], status: Schemas["TeamStatus"] = "active"): Schemas["TeamMember"] => ({
  id, name, email: `${name.toLowerCase()}@contoh.my`, role, teamId: null, status, lastSignInAt: null, createdAt: "2026-11-01T00:00:00Z",
});
const reads = () => vi.spyOn(api, "GET").mockResolvedValue(ok({
  items: [member("usr_me", "Aina", "admin"), member("usr_o", "Olivia", "owner"), member("usr_r", "Rizal", "recruiter"), member("usr_n", "Nur", "sourcer", "invited")],
}));

describe("team", () => {
  it("lets admins change others, but not themselves or owners", async () => {
    reads();
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(member("usr_r", "Rizal", "sourcer")));
    renderWithClient(<Team myRole="admin" myId="usr_me" />);
    const rizal = await screen.findByLabelText("Role of Rizal");
    expect(screen.queryByLabelText("Role of Aina")).toBeNull();
    expect(screen.queryByLabelText("Role of Olivia")).toBeNull();
    expect(within(rizal).queryByText("Owner")).toBeNull(); // admins do not make owners
    fireEvent.change(rizal, { target: { value: "sourcer" } });
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ role: "sourcer" });
    expect(screen.getByRole("button", { name: "Send new invitation" })).toBeTruthy();
  });

  it("invites someone and shows the link to pass on", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({
      member: member("usr_x", "Wei", "recruiter", "invited"), link: "http://localhost:5173/#/invite/abc", expiresAt: "2026-11-10T00:00:00Z", emailed: true,
    }));
    renderWithClient(<Team myRole="admin" myId="usr_me" />);
    const form = await screen.findByRole("region", { name: "Invite someone" });
    fireEvent.change(within(form).getByLabelText("Email"), { target: { value: " wei@contoh.my " } });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Wei" } });
    fireEvent.click(within(form).getByRole("button", { name: "Invite" }));
    expect(await screen.findByText("http://localhost:5173/#/invite/abc")).toBeTruthy();
    expect(sentBody(post)).toEqual({ email: "wei@contoh.my", name: "Wei", role: "recruiter" });
  });

  it("puts people in teams, and adds teams", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/teams"
      ? { items: [{ id: "team_1", version: 1, name: "Finance desk", memberCount: 0 }] }
      : { items: [member("usr_me", "Aina", "owner"), member("usr_r", "Rizal", "recruiter")] }))) as never);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...member("usr_r", "Rizal", "recruiter"), teamId: "team_1" }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "team_2", version: 1, name: "Tech desk", memberCount: 0 }));
    renderWithClient(<Team myRole="owner" myId="usr_me" />);
    expect(screen.getByRole("link", { name: "Roles and access" })).toBeTruthy();
    const team = await screen.findByLabelText<HTMLSelectElement>("Team of Rizal");
    await waitFor(() => { expect(team.options.length).toBe(2); });
    fireEvent.change(team, { target: { value: "team_1" } });
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ teamId: "team_1" });

    fireEvent.change(screen.getByLabelText("New team"), { target: { value: " Tech desk " } });
    fireEvent.click(screen.getByRole("button", { name: "Add team" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ name: "Tech desk" });
  });
});
