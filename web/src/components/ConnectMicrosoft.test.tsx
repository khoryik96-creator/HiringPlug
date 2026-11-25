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

  it("connects an owner's account: this tab keeps that it is connecting, and sends only the value's SHA-256", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://login.microsoftonline.com/org/oauth2/v2.0/authorize?client_id=c" }));
    const go = vi.spyOn(browser, "go").mockImplementation(() => undefined);
    renderWithClient(<ConnectMicrosoft me={me({ role: "owner" })} />);
    expect(screen.getByText(/As an owner, you connect it here, once/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Connect Microsoft sign-in" }));
    await waitFor(() => { expect(go).toHaveBeenCalledWith("https://login.microsoftonline.com/org/oauth2/v2.0/authorize?client_id=c"); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/me/microsoft/start");
    expect(sentBody(post)).toEqual({ bindingHash: await sha256(takeBinding() ?? "") });
    expect(isConnecting()).toBe(true);
    expect(screen.getByRole("button", { name: /Opening Microsoft/ })).toBeTruthy();
  });

  it("says so when one is connected already", () => {
    renderWithClient(<ConnectMicrosoft me={me({ microsoftLinked: true })} />);
    expect(screen.getByText("Connected")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Connect Microsoft sign-in" })).toBeNull();
    expect(screen.getByText(/ask an owner to unlink this one on the Team page/)).toBeTruthy();
  });

  it("shows why it could not start", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_SIGN_IN_UNAVAILABLE", "Signing in with Microsoft is not set up here.", undefined, 422));
    renderWithClient(<ConnectMicrosoft me={me({})} />);
    expect(screen.queryByText(/As an owner/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Connect Microsoft sign-in" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Signing in with Microsoft is not set up here.");
  });
});
