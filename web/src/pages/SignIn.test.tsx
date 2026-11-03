import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { authLinkRoute, getToken } from "../auth";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { ChoosePassword } from "./ChoosePassword";
import { SignIn } from "./SignIn";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

const fill = (label: string, value: string) => { fireEvent.change(screen.getByLabelText(label), { target: { value } }); };

describe("signing in", () => {
  it("signs in with email and password", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ token: "tok_1", expiresAt: "2026-11-04T00:00:00Z" }));
    const onSignedIn = vi.fn();
    renderWithClient(<SignIn onSignedIn={onSignedIn} />);
    fill("Email", " siti@contoh.my ");
    fill("Password", "purple-gecko-41");
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => { expect(onSignedIn).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ email: "siti@contoh.my", password: "purple-gecko-41" });
    expect(getToken()).toBe("tok_1");
  });

  it("asks which company when the account is at several", async () => {
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(failure("SIGN_IN_CHOOSE_COMPANY", "Choose one.", { companies: [{ slug: "alpha", name: "Alpha" }, { slug: "beta", name: "Beta" }] }, 409))
      .mockResolvedValueOnce(ok({ token: "tok_2", expiresAt: "2026-11-04T00:00:00Z" }));
    renderWithClient(<SignIn onSignedIn={vi.fn()} />);
    fill("Email", "sam@x.my");
    fill("Password", "purple-gecko-41");
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    fireEvent.click(await screen.findByLabelText("Beta"));
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post, 1)).toEqual({ email: "sam@x.my", password: "purple-gecko-41", company: "beta" });
  });

  it("asks for a reset link without saying whether the email exists", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 202 }) });
    renderWithClient(<SignIn onSignedIn={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Forgot your password?" }));
    fill("Email", "siti@contoh.my");
    fireEvent.click(screen.getByRole("button", { name: "Send link" }));
    expect((await screen.findByRole("status")).textContent).toContain("If siti@contoh.my belongs to an account");
    expect(sentBody(post)).toEqual({ email: "siti@contoh.my" });
  });

  it("joins with an invitation link", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ kind: "invite", email: "nur@contoh.my", name: "Nur", companyName: "Contoh Recruitment" }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ token: "tok_3", expiresAt: "2026-11-04T00:00:00Z" }));
    const onSignedIn = vi.fn();
    renderWithClient(<ChoosePassword token={"a".repeat(64)} onSignedIn={onSignedIn} />);
    expect(await screen.findByText("Contoh Recruitment")).toBeTruthy();
    fill("Password (at least 10 characters)", "purple-gecko-41");
    fill("The same again", "purple-gecko-4");
    expect(screen.getByRole("status").textContent).toContain("differ");
    fill("The same again", "purple-gecko-41");
    fireEvent.click(screen.getByRole("button", { name: "Join" }));
    await waitFor(() => { expect(onSignedIn).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ password: "purple-gecko-41" });
    expect(getToken()).toBe("tok_3");
  });

  it("recognises link addresses", () => {
    expect(authLinkRoute("#/invite/" + "b".repeat(64))).toEqual({ kind: "invite", token: "b".repeat(64) });
    expect(authLinkRoute("#/reset/short")).toBeNull();
    expect(authLinkRoute("#/candidates")).toBeNull();
  });
});
