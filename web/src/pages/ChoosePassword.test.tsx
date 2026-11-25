import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { browser, takeBinding } from "../microsoft";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { ChoosePassword } from "./ChoosePassword";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

const LINK = "a".repeat(64);
const link = (kind: "invite" | "reset", microsoft: "none" | "available" | "required") =>
  ok({ kind, email: "nur@contoh.my", name: "Nur", companyName: "Contoh Recruitment", microsoft });

describe("invitation and reset links with Microsoft", () => {
  it("offers Accept with Microsoft beside choosing a password, started with the invitation", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(link("invite", "available"));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://login.example/authorize" }));
    const go = vi.spyOn(browser, "go").mockImplementation(() => undefined);
    renderWithClient(<ChoosePassword token={LINK} onSignedIn={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Accept with Microsoft" }));
    expect(screen.getByLabelText("Password (at least 10 characters)")).toBeTruthy();
    expect(screen.getByText("or choose a password")).toBeTruthy();
    await waitFor(() => { expect(go).toHaveBeenCalledWith("https://login.example/authorize"); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/auth/microsoft/start");
    expect(sentBody(post)).toEqual({ bindingHash: expect.stringMatching(/^[0-9a-f]{64}$/) as unknown, invite: LINK });
    expect(takeBinding()).toMatch(/^[0-9a-f]{64}$/);
  });

  it("hides the password when the company requires Microsoft", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(link("invite", "required"));
    renderWithClient(<ChoosePassword token={LINK} onSignedIn={vi.fn()} />);
    expect(await screen.findByRole("button", { name: "Accept with Microsoft" })).toBeTruthy();
    expect(screen.getByText(/signs in with Microsoft: accept with your work account/)).toBeTruthy();
    expect(screen.queryByLabelText("Password (at least 10 characters)")).toBeNull();
    expect(screen.queryByRole("button", { name: "Join" })).toBeNull();
  });

  it("has no Microsoft button when the company does not sign in with Microsoft", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(link("invite", "none"));
    renderWithClient(<ChoosePassword token={LINK} onSignedIn={vi.fn()} />);
    expect(await screen.findByRole("button", { name: "Join" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Accept with Microsoft" })).toBeNull();
  });

  it("says a reset link has no password to set while Microsoft is required", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(link("reset", "required"));
    renderWithClient(<ChoosePassword token={LINK} onSignedIn={vi.fn()} />);
    expect(await screen.findByText("Your company signs in with Microsoft: use Sign in with Microsoft.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Sign in with Microsoft" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save password" })).toBeNull();
  });

  it("shows the server's words when Microsoft became required meanwhile", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(link("reset", "none"));
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_REQUIRED", "Your company signs in with Microsoft: use Sign in with Microsoft.", undefined, 403));
    renderWithClient(<ChoosePassword token={LINK} onSignedIn={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText("Password (at least 10 characters)"), { target: { value: "purple-gecko-41" } });
    fireEvent.change(screen.getByLabelText("The same again"), { target: { value: "purple-gecko-41" } });
    fireEvent.click(screen.getByRole("button", { name: "Save password" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Your company signs in with Microsoft");
  });
});
