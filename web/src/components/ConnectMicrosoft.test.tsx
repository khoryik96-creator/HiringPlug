import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { browser, isConnecting, takeBinding } from "../microsoft";
import { failure, ok, renderWithClient, sentBody, testMe } from "../test-utils";
import { ConnectMicrosoft } from "./ConnectMicrosoft";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

const me = (over: Partial<Schemas["Me"]>): Schemas["Me"] => ({ ...testMe([]), microsoftSignIn: true, ...over });

async function sha256(text: string): Promise<string> {
  const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(d)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

describe("Your account › Microsoft sign-in", () => {
  it("is not there when the company does not sign in with Microsoft", () => {
    renderWithClient(<ConnectMicrosoft me={me({ microsoftSignIn: false })} />);
    expect(screen.queryByRole("region", { name: "Microsoft sign-in" })).toBeNull();
  });

  const twoStep = (enabled: boolean) => vi.spyOn(api, "GET").mockResolvedValue(ok({ enabled, required: false, enabledAt: null, recoveryCodesLeft: 0 }));

  it("connects an owner's account with their password and two-step code: this tab keeps that it is connecting, and sends only the value's SHA-256", async () => {
    twoStep(true);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://login.microsoftonline.com/org/oauth2/v2.0/authorize?client_id=c" }));
    const go = vi.spyOn(browser, "go").mockImplementation(() => undefined);
    renderWithClient(<ConnectMicrosoft me={me({ role: "owner" })} />);
    expect(screen.getByText(/As an owner, you connect it here, once/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Connect Microsoft sign-in" }));
    expect(post).not.toHaveBeenCalled(); // asks who you are first
    fireEvent.change(screen.getByLabelText("Your current password"), { target: { value: "my-password-1" } });
    fireEvent.change(await screen.findByLabelText(/Two-step code/), { target: { value: " 123456 " } });
    fireEvent.click(screen.getByRole("button", { name: "Continue to Microsoft" }));
    await waitFor(() => { expect(go).toHaveBeenCalledWith("https://login.microsoftonline.com/org/oauth2/v2.0/authorize?client_id=c"); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/me/microsoft/start");
    expect(sentBody(post)).toEqual({ bindingHash: await sha256(takeBinding() ?? ""), password: "my-password-1", code: "123456" });
    expect(isConnecting()).toBe(true);
    expect(screen.getByRole("button", { name: /Opening Microsoft/ })).toBeTruthy();
  });

  it("says so when one is connected already", () => {
    renderWithClient(<ConnectMicrosoft me={me({ microsoftLinked: true })} />);
    expect(screen.getByText("Connected")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Connect Microsoft sign-in" })).toBeNull();
    expect(screen.getByText(/ask an owner to unlink this one on the Team page/)).toBeTruthy();
  });

  it("asks no code of someone without two-step sign-in, and says a wrong password is wrong", async () => {
    twoStep(false);
    vi.spyOn(api, "POST").mockResolvedValue(failure("VALIDATION_FAILED", "Some fields need attention.", { fields: [{ field: "password", message: "is not your current password" }] }, 400));
    renderWithClient(<ConnectMicrosoft me={me({})} />);
    fireEvent.click(screen.getByRole("button", { name: "Connect Microsoft sign-in" }));
    fireEvent.change(screen.getByLabelText("Your current password"), { target: { value: "wrong" } });
    await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button", { name: "Continue to Microsoft" }).disabled).toBe(false); });
    expect(screen.queryByLabelText(/Two-step code/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Continue to Microsoft" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Password is not your current password");
  });

  it("asks no password of someone who has none", async () => {
    twoStep(false);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://login.microsoftonline.com/x" }));
    vi.spyOn(browser, "go").mockImplementation(() => undefined);
    renderWithClient(<ConnectMicrosoft me={me({ hasPassword: false })} />);
    fireEvent.click(screen.getByRole("button", { name: "Connect Microsoft sign-in" }));
    expect(screen.queryByLabelText("Your current password")).toBeNull();
    await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button", { name: "Continue to Microsoft" }).disabled).toBe(false); });
    fireEvent.click(screen.getByRole("button", { name: "Continue to Microsoft" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(Object.keys(sentBody(post) as object)).toEqual(["bindingHash"]);
  });

  it("shows why it could not start", async () => {
    twoStep(false);
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_SIGN_IN_UNAVAILABLE", "Signing in with Microsoft is not set up here.", undefined, 422));
    renderWithClient(<ConnectMicrosoft me={me({ hasPassword: false })} />);
    expect(screen.queryByText(/As an owner/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Connect Microsoft sign-in" }));
    await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button", { name: "Continue to Microsoft" }).disabled).toBe(false); });
    fireEvent.click(screen.getByRole("button", { name: "Continue to Microsoft" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Signing in with Microsoft is not set up here.");
  });
});
