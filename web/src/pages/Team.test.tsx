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
  id, name, email: `${name.toLowerCase()}@contoh.my`, role, status, lastSignInAt: null, createdAt: "2026-11-01T00:00:00Z",
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
});
