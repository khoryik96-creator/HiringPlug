import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ChangePassword, Team } from "./Team";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const member = (id: string, name: string, role: Schemas["Role"], status: Schemas["TeamStatus"] = "active"): Schemas["TeamMember"] => ({
  id, name, email: `${name.toLowerCase()}@contoh.my`, role, teamId: null, status, twoStep: false, signInMethods: ["password"], lastSignInAt: null, createdAt: "2026-11-01T00:00:00Z",
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

  it("unlinks a member's Microsoft account, after asking; owners' only for owners", async () => {
    const withMicrosoft = (m: Schemas["TeamMember"]): Schemas["TeamMember"] => ({ ...m, signInMethods: ["password", "microsoft"] });
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({
      items: [member("usr_me", "Aina", "admin"), withMicrosoft(member("usr_o", "Olivia", "owner")), withMicrosoft(member("usr_r", "Rizal", "recruiter")), member("usr_s", "Siti", "sourcer")],
    }));
    const del = vi.spyOn(api, "DELETE").mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 204 }) });
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<Team myRole="admin" myId="usr_me" />);
    const buttons = await screen.findAllByRole("button", { name: "Unlink Microsoft" });
    expect(buttons).toHaveLength(1); // Rizal's: not the owner's (admins), nor Siti's (none linked)
    fireEvent.click(buttons[0] as HTMLElement);
    expect(confirm.mock.calls[0]?.[0]).toContain("Unlink Rizal's Microsoft account? Their sessions signed in with Microsoft end now.");
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
    expect(del.mock.calls[0]?.[0]).toBe("/v1/team/{userId}/microsoft");
    expect(del.mock.calls[0]?.[1]).toEqual({ params: { path: { userId: "usr_r" } } });
    cleanup();

    get.mockClear();
    confirm.mockReturnValue(false);
    renderWithClient(<Team myRole="owner" myId="usr_me" />);
    expect(await screen.findAllByRole("button", { name: "Unlink Microsoft" })).toHaveLength(2); // owners unlink owners too
    fireEvent.click(screen.getAllByRole("button", { name: "Unlink Microsoft" })[0] as HTMLElement);
    expect(confirm.mock.calls.at(-1)?.[0]).toContain("connected to them only on purpose");
    expect(del).toHaveBeenCalledTimes(1); // not confirmed: nothing unlinked
  });

  it("keeps each person's actions in a menu, and their email under their name", async () => {
    reads();
    renderWithClient(<Team myRole="owner" myId="usr_me" />);
    const menu = await screen.findByLabelText("Actions for Rizal");
    expect(menu.tagName).toBe("SUMMARY");
    expect(screen.queryByRole("columnheader", { name: "Email" })).toBeNull();
    const row = menu.closest("tr") as HTMLElement;
    expect(within(row).getByText("rizal@contoh.my")).toBeTruthy();
    expect(within(row).getByText("Never signed in")).toBeTruthy();
    expect(screen.queryByLabelText("Actions for Aina")).toBeNull(); // nothing to do to yourself here
  });

  it("emails someone a link to reconnect Microsoft, and shows it to pass on", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({
      items: [member("usr_me", "Aina", "admin"), member("usr_o", "Olivia", "owner"), member("usr_r", "Rizal", "recruiter"),
        { ...member("usr_l", "Lina", "recruiter"), signInMethods: ["microsoft"] }, member("usr_n", "Nur", "sourcer", "invited")],
    }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ expiresAt: "2026-12-02T00:00:00Z", emailed: true }));
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<Team myRole="admin" myId="usr_me" microsoft />);
    const buttons = await screen.findAllByRole("button", { name: "Reconnect Microsoft" });
    expect(buttons).toHaveLength(1); // Rizal's: not Aina's (her own: Your account), the owner's (admins), Lina's (linked) nor Nur's (invited)
    fireEvent.click(within(screen.getByLabelText("Actions for Rizal").closest("tr") as HTMLElement).getByRole("button", { name: "Reconnect Microsoft" }));
    expect(confirm.mock.calls[0]?.[0]).toContain("whatever its address");
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/team/{userId}/microsoft/reconnect");
    expect(post.mock.calls[0]?.[1]).toEqual({ params: { path: { userId: "usr_r" } } });
    expect((await screen.findByRole("status")).textContent).toContain("Link to connect Microsoft emailed to rizal@contoh.my. It works once, for 7 days.");
    cleanup();
    get.mockClear();
    renderWithClient(<Team myRole="admin" myId="usr_me" />);
    await screen.findByLabelText("Actions for Rizal");
    expect(screen.queryByRole("button", { name: "Reconnect Microsoft" }), "the company does not sign in with Microsoft").toBeNull();
  });

  it("cancels an invitation sent to the wrong person, after asking", async () => {
    reads();
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok(undefined));
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<Team myRole="admin" myId="usr_me" />);
    fireEvent.click(await screen.findByRole("button", { name: "Cancel invitation" }));
    expect(confirm.mock.calls[0]?.[0]).toContain("Their link stops working now");
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
    expect(del.mock.calls[0]?.[0]).toBe("/v1/team/{userId}/invite");
    expect(del.mock.calls[0]?.[1]).toEqual({ params: { path: { userId: "usr_n" } } });
  });

  it("offers nobody a role above their own, and leaves admins to admins", async () => {
    reads();
    renderWithClient(<Team myRole="recruiter" myId="usr_r" />);
    const nur = await screen.findByLabelText("Role of Nur");
    expect(within(nur).queryByText("Admin"), "a recruiter let to manage the team does not make admins").toBeNull();
    expect(within(nur).getByText("Sourcer")).toBeTruthy();
    expect(screen.queryByLabelText("Role of Aina"), "nor change one").toBeNull();
  });

  it("invites someone and shows the link to pass on", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({
      member: member("usr_x", "Wei", "recruiter", "invited"), link: "http://localhost:5173/#/invite/abc", expiresAt: "2026-11-10T00:00:00Z", emailed: true,
      delivery: "server",
    }));
    renderWithClient(<Team myRole="admin" myId="usr_me" />);
    const form = await screen.findByRole("region", { name: "Invite someone" });
    fireEvent.change(within(form).getByLabelText("Email"), { target: { value: " wei@contoh.my " } });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Wei" } });
    fireEvent.click(within(form).getByRole("button", { name: "Invite" }));
    expect(await screen.findByText("http://localhost:5173/#/invite/abc")).toBeTruthy();
    expect(sentBody(post)).toEqual({ email: "wei@contoh.my", name: "Wei", role: "recruiter" });
  });

  it("shows how each invitation email went: sent, or failed with the mail server's answer", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      member("usr_me", "Aina", "owner"),
      { ...member("usr_j", "Jerome", "owner", "invited"), invitationEmail: { status: "failed", error: "550 5.7.1 Client does not have permissions to send as this sender", at: "2026-11-27T01:00:00Z" } },
      { ...member("usr_t", "Tan", "recruiter", "invited"), invitationEmail: { status: "sent", error: null, at: "2026-11-27T01:00:00Z" } },
    ] }));
    renderWithClient(<Team myRole="owner" myId="usr_me" />);
    expect((await screen.findByText(/Invitation email failed/)).closest("[role=note]")?.textContent)
      .toContain("550 5.7.1 Client does not have permissions to send as this sender");
    expect(screen.getByText(/Invitation email sent/)).toBeTruthy();
  });

  it("says plainly when an invitation could not be emailed, and how to fix it", async () => {
    reads();
    vi.spyOn(api, "POST").mockResolvedValue(ok({
      member: member("usr_x", "Wei", "recruiter", "invited"), link: "http://localhost:5173/#/invite/abc", expiresAt: "2026-11-10T00:00:00Z", emailed: false,
      delivery: "none",
    }));
    renderWithClient(<Team myRole="admin" myId="usr_me" />);
    const form = await screen.findByRole("region", { name: "Invite someone" });
    fireEvent.change(within(form).getByLabelText("Email"), { target: { value: "wei@contoh.my" } });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Wei" } });
    fireEvent.click(within(form).getByRole("button", { name: "Invite" }));
    const note = await screen.findByRole("status");
    expect(note.textContent).toMatch(/Not emailed: the server's email is not set up and you have no mailbox connected/);
    expect(note.textContent).not.toContain("Invitation emailed");
    expect(within(note).getByRole("link", { name: "Settings › Email connection" }).getAttribute("href")).toBe("#/email-connection");
    expect(within(note).getByText("http://localhost:5173/#/invite/abc")).toBeTruthy();
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

describe("how each person signs in", () => {
  it("shows Microsoft and/or Password beside each name, and nothing while invited", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      { ...member("usr_me", "Aina", "owner"), signInMethods: ["password", "microsoft"] },
      { ...member("usr_r", "Rizal", "recruiter"), signInMethods: ["microsoft"] },
      member("usr_s", "Sam", "sourcer"),
      { ...member("usr_n", "Nur", "sourcer", "invited"), signInMethods: [] },
    ] }));
    renderWithClient(<Team myRole="owner" myId="usr_me" />);
    await screen.findByLabelText("Role of Rizal");
    const row = (name: string) => Promise.resolve(screen.getAllByRole("row").find((r) => r.textContent.startsWith(name)) as HTMLElement);
    const badges = async (name: string) => within(await row(name)).queryAllByText(/^\s*(Microsoft|Password)$/).map((b) => b.textContent.trim());
    expect(await badges("Aina")).toEqual(["Microsoft", "Password"]);
    expect(await badges("Rizal")).toEqual(["Microsoft"]);
    expect(await badges("Sam")).toEqual(["Password"]);
    expect(await badges("Nur")).toEqual([]);
    expect(within(await row("Rizal")).getByLabelText("Signs in with Microsoft")).toBeTruthy();
  });
});

describe("your password", () => {
  it("says there is none to change for someone who signs in with Microsoft", () => {
    renderWithClient(<ChangePassword hasPassword={false} />);
    expect(screen.getByText(/You sign in with Microsoft/)).toBeTruthy();
    expect(screen.queryByLabelText("Current password")).toBeNull();
    expect(screen.queryByRole("button", { name: "Change password" })).toBeNull();
    cleanup();
    renderWithClient(<ChangePassword hasPassword />);
    expect(screen.getByLabelText("Current password")).toBeTruthy();
  });
});
