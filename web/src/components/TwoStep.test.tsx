import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, ApiError, type Schemas } from "../api/client";
import { getToken } from "../auth";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { MyTwoStep, nextStepOf, TwoStepAtSignIn } from "./TwoStep";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

const fill = (label: string, value: string) => { fireEvent.change(screen.getByLabelText(label), { target: { value } }); };
const setup = { secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", uri: "otpauth://totp/HiringPlug%20%28Contoh%29:siti@contoh.my?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&issuer=HiringPlug" };
const session = { token: "tok_9", expiresAt: "2026-11-23T00:00:00Z" };

describe("two-step sign-in", () => {
  const err = (code: Schemas["ErrorCode"], details: Record<string, unknown>) => new ApiError({ error: { code, message: "x", requestId: "r", details } });

  it("reads the next step from a sign-in error", () => {
    expect(nextStepOf(err("TWO_STEP_REQUIRED", { challenge: "c1" }))).toEqual({ kind: "code", challenge: "c1" });
    expect(nextStepOf(err("TWO_STEP_SETUP_REQUIRED", { challenge: "c2" }))).toEqual({ kind: "setup", challenge: "c2" });
    expect(nextStepOf(err("UNAUTHENTICATED", {}))).toBeNull();
    expect(nextStepOf(err("TWO_STEP_REQUIRED", {}))).toBeNull();
  });

  it("signs in with the code from the app", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(session));
    const onSignedIn = vi.fn();
    renderWithClient(<TwoStepAtSignIn next={{ kind: "code", challenge: "c1" }} onSignedIn={onSignedIn} onCancel={vi.fn()} />);
    fill("Code from the app, or a recovery code", "287082");
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => { expect(onSignedIn).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ challenge: "c1", code: "287082" });
    expect(getToken()).toBe("tok_9");
  });

  it("sets the app up when the company requires it, showing recovery codes before going on", async () => {
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(ok(setup))
      .mockResolvedValueOnce(ok({ session, recoveryCodes: ["abcde-fghjk", "mnpqr-stuvw"] }));
    const onSignedIn = vi.fn();
    renderWithClient(<TwoStepAtSignIn next={{ kind: "setup", challenge: "c2" }} onSignedIn={onSignedIn} onCancel={vi.fn()} />);
    expect(await screen.findByRole("img", { name: "QR code for your authenticator app" })).toBeTruthy();
    expect(screen.getByText("GEZD GNBV GY3T QOJQ GEZD GNBV GY3T QOJQ")).toBeTruthy();
    expect(sentBody(post)).toEqual({ challenge: "c2" });
    fill("Code from the app", "287082");
    fireEvent.click(screen.getByRole("button", { name: "Turn on and sign in" }));
    expect(await screen.findByText("abcde-fghjk")).toBeTruthy();
    expect(sentBody(post, 1)).toEqual({ challenge: "c2", code: "287082" });
    expect(onSignedIn).not.toHaveBeenCalled();
    expect(getToken()).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onSignedIn).toHaveBeenCalled();
    expect(getToken()).toBe("tok_9");
  });
});

describe("your two-step sign-in", () => {
  it("sets it up from Your account", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ enabled: false, recoveryCodesLeft: 0, required: false }));
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(ok(setup))
      .mockResolvedValueOnce(ok({ recoveryCodes: ["abcde-fghjk"] }));
    renderWithClient(<MyTwoStep />);
    fireEvent.click(await screen.findByRole("button", { name: "Set up" }));
    expect(await screen.findByRole("img", { name: "QR code for your authenticator app" })).toBeTruthy();
    fill("Code from the app", "123456");
    fireEvent.click(screen.getByRole("button", { name: "Turn on" }));
    expect(await screen.findByText("abcde-fghjk")).toBeTruthy();
    expect(sentBody(post, 1)).toEqual({ code: "123456" });
  });

  it("turns it off with the password, unless the company requires it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ enabled: true, enabledAt: "2026-11-20T00:00:00Z", recoveryCodesLeft: 1, required: false }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
    renderWithClient(<MyTwoStep />);
    expect(await screen.findByText(/1 recovery code left/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Turn off" }));
    fill("Your password", "purple-gecko-41");
    fireEvent.click(screen.getByRole("button", { name: "Turn off" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ password: "purple-gecko-41" });
  });

  it("turns it off with the app's code for someone without a password", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ enabled: true, enabledAt: "2026-11-20T00:00:00Z", recoveryCodesLeft: 3, required: false }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
    renderWithClient(<MyTwoStep hasPassword={false} />);
    fireEvent.click(await screen.findByRole("button", { name: "Turn off" }));
    expect(screen.queryByLabelText("Your password")).toBeNull();
    fill("Code from the app", "287082");
    fireEvent.click(screen.getByRole("button", { name: "Turn off" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ code: "287082" });
  });

  it("speaks of Microsoft, not a stolen password, to someone without a password", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ enabled: false, enabledAt: null, recoveryCodesLeft: 0, required: false }));
    renderWithClient(<MyTwoStep hasPassword={false} />);
    const off = await screen.findByText(/You sign in with Microsoft/);
    expect(off.textContent).toContain("HiringPlug's code is not asked then");
    expect(screen.queryByText(/stolen password/)).toBeNull();
    cleanup();
    get.mockResolvedValue(ok({ enabled: true, enabledAt: "2026-11-20T00:00:00Z", recoveryCodesLeft: 4, required: false }));
    renderWithClient(<MyTwoStep hasPassword={false} />);
    const on = await screen.findByText(/You sign in with Microsoft/);
    expect(on.textContent).toContain("asked only if you ever sign in with a password");
    expect(on.textContent).toContain("4 recovery codes left");
    expect(screen.queryByText(/after your password/)).toBeNull();
    cleanup();
    get.mockResolvedValue(ok({ enabled: false, enabledAt: null, recoveryCodesLeft: 0, required: false }));
    renderWithClient(<MyTwoStep />);
    expect(await screen.findByText(/a stolen password alone cannot sign in as you/)).toBeTruthy();
  });

  it("has no Turn off while the company requires it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ enabled: true, enabledAt: "2026-11-20T00:00:00Z", recoveryCodesLeft: 10, required: true }));
    renderWithClient(<MyTwoStep />);
    expect(await screen.findByText(/Your company requires it/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Turn off" })).toBeNull();
  });
});
