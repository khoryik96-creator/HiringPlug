import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { getToken } from "../auth";
import { browser, microsoftRoute, takeBinding } from "../microsoft";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { MicrosoftSignIn, NOT_CONNECTED, REFUSED } from "./MicrosoftSignIn";
import { SignIn } from "./SignIn";

const session = { token: "tok_ms", expiresAt: "2026-11-26T00:00:00Z" };
const BINDING = "hiringplug.microsoft.binding";
const TICKET = "t".repeat(43);
const VALUE = "b".repeat(64);

async function sha256(text: string): Promise<string> {
  const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(d)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** The address Microsoft's sign-in comes back to, with this tab's value kept (or not). */
function arrive(query: string, binding: string | null = VALUE) {
  window.history.replaceState(null, "", `/#/sign-in/microsoft?${query}`);
  if (binding) sessionStorage.setItem(BINDING, binding);
}

/** Back from connecting an account from Your account, signed in in this tab. */
function arriveConnecting(query: string) {
  arrive(query);
  sessionStorage.setItem("hiringplug.microsoft.connecting", "1");
  sessionStorage.setItem("hiringplug.token", "tok_signed_in");
}

beforeEach(() => { window.history.replaceState(null, "", "/"); });
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

describe("starting with Microsoft", () => {
  it("keeps a random value in this tab and sends only its SHA-256, with the email typed so far", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://login.microsoftonline.com/x/oauth2/v2.0/authorize?client_id=c" }));
    const go = vi.spyOn(browser, "go").mockImplementation(() => undefined);
    renderWithClient(<SignIn onSignedIn={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: " siti@contoh.my " } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in with Microsoft" }));
    await waitFor(() => { expect(go).toHaveBeenCalledWith("https://login.microsoftonline.com/x/oauth2/v2.0/authorize?client_id=c"); });
    const value = takeBinding() ?? "";
    expect(value).toMatch(/^[0-9a-f]{64}$/);
    expect(post.mock.calls[0]?.[0]).toBe("/v1/auth/microsoft/start");
    expect(sentBody(post)).toEqual({ bindingHash: await sha256(value), email: "siti@contoh.my" });
    expect(screen.getByRole("button", { name: /Opening Microsoft/ })).toBeTruthy();
  });

  it("leaves out an email that is not one yet, and a new start makes a new value", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://login.example/a" }));
    vi.spyOn(browser, "go").mockImplementation(() => undefined);
    sessionStorage.setItem(BINDING, VALUE);
    renderWithClient(<SignIn onSignedIn={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "siti" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in with Microsoft" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(Object.keys(sentBody(post) as object)).toEqual(["bindingHash"]);
    expect(takeBinding()).not.toBe(VALUE);
  });

  it("says calmly when Microsoft is not set up", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_SIGN_IN_UNAVAILABLE", "Not set up.", undefined, 422));
    const go = vi.spyOn(browser, "go");
    renderWithClient(<SignIn onSignedIn={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Sign in with Microsoft" }));
    expect((await screen.findByRole("status")).textContent).toContain("isn't set up for your company yet");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(go).not.toHaveBeenCalled();
  });

  it("shows the server's words when the company requires Microsoft and a password is tried", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_REQUIRED", "Your company signs in with Microsoft: use Sign in with Microsoft.", undefined, 403));
    renderWithClient(<SignIn onSignedIn={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "siti@contoh.my" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "purple-gecko-41" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Your company signs in with Microsoft: use Sign in with Microsoft.");
  });

  it("keeps the developer token option", () => {
    renderWithClient(<SignIn onSignedIn={vi.fn()} />);
    expect(screen.getByText("Sign in with a token (developers)")).toBeTruthy();
  });
});

describe("back from Microsoft", () => {
  it("recognises the address", () => {
    expect(microsoftRoute(`#/sign-in/microsoft?ticket=${TICKET}`)).toEqual({ ticket: TICKET, error: "" });
    expect(microsoftRoute("#/sign-in/microsoft?error=cancelled")).toEqual({ ticket: "", error: "cancelled" });
    expect(microsoftRoute("#/sign-in/microsoft")).toEqual({ ticket: "", error: "" });
    expect(microsoftRoute("#/sign-in/microsoftx")).toBeNull();
    expect(microsoftRoute("#/candidates")).toBeNull();
  });

  it("finishes with the ticket and this tab's value, keeps the sign-in, and forgets both", async () => {
    arrive(`ticket=${TICKET}`);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(session));
    const onSignedIn = vi.fn();
    renderWithClient(<MicrosoftSignIn onSignedIn={onSignedIn} />);
    await waitFor(() => { expect(onSignedIn).toHaveBeenCalled(); });
    expect(post).toHaveBeenCalledTimes(1);
    expect(post.mock.calls[0]?.[0]).toBe("/v1/auth/microsoft/complete");
    expect(sentBody(post)).toEqual({ ticket: TICKET, binding: VALUE });
    expect(getToken()).toBe("tok_ms");
    expect(takeBinding()).toBeNull();
    expect(window.location.hash).toBe("#/");
    expect(window.location.href).not.toContain(TICKET);
  });

  it.each(Object.keys(REFUSED))("says what %s means, forgets the value and offers the way back", async (reason) => {
    arrive(`ticket=${TICKET}`);
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_SIGN_IN_FAILED", "server words", { reason }, 401));
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toBe(REFUSED[reason]);
    expect(screen.getByRole("link", { name: "Back to sign in" }).getAttribute("href")).toBe("#/");
    expect(takeBinding()).toBeNull();
    expect(getToken()).toBeNull();
    expect(window.location.hash).toBe("#/sign-in/microsoft"); // the ticket is gone from the address
  });

  it("tells someone not on the team to ask an owner", async () => {
    arrive(`ticket=${TICKET}`);
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_SIGN_IN_FAILED", "x", { reason: "not_on_team" }, 401));
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toBe("Your Microsoft account isn't on this HiringPlug team yet. Ask an owner to invite you.");
  });

  it("asks which company for an account at several, and finishes with the new ticket", async () => {
    arrive(`ticket=${TICKET}`);
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(failure("SIGN_IN_CHOOSE_COMPANY", "Choose.", { companies: [{ slug: "alpha", name: "Alpha" }, { slug: "beta", name: "Beta" }], ticket: "n".repeat(43) }, 409))
      .mockResolvedValueOnce(ok(session));
    const onSignedIn = vi.fn();
    renderWithClient(<MicrosoftSignIn onSignedIn={onSignedIn} />);
    expect(await screen.findByText("Your Microsoft account is at several companies. Which one?")).toBeTruthy();
    expect(takeBinding()).toBe(VALUE); // still needed for the second ticket
    fireEvent.click(screen.getByRole("button", { name: "Beta" }));
    await waitFor(() => { expect(onSignedIn).toHaveBeenCalled(); });
    expect(sentBody(post, 1)).toEqual({ ticket: "n".repeat(43), binding: VALUE, company: "beta" });
    expect(getToken()).toBe("tok_ms");
    expect(takeBinding()).toBeNull();
  });

  it("says when the sign-in started in another browser, without asking the server", async () => {
    arrive(`ticket=${TICKET}`, null);
    const post = vi.spyOn(api, "POST");
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toContain("started in another browser or tab");
    expect(screen.getByRole("link", { name: "Back to sign in" })).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
    expect(window.location.hash).toBe("#/sign-in/microsoft");
  });

  it("says when it was cancelled at Microsoft, and forgets the value", async () => {
    arrive("error=cancelled");
    const post = vi.spyOn(api, "POST");
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toContain("You cancelled signing in with Microsoft");
    expect(screen.getByRole("link", { name: "Back to sign in" })).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
    expect(takeBinding()).toBeNull();
  });

  it("says when Microsoft did not finish, or the ticket is gone", async () => {
    arrive("error=failed");
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toContain("Microsoft didn't finish signing you in");
    cleanup();
    arrive("");
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toContain("already finished or has expired");
  });

  it("explains too many attempts with the server's message", async () => {
    arrive(`ticket=${TICKET}`);
    vi.spyOn(api, "POST").mockResolvedValue(failure("TOO_MANY_ATTEMPTS", "Too many attempts; wait a few minutes.", undefined, 429));
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toContain("Too many attempts; wait a few minutes.");
  });
});

describe("back from connecting an account (Your account)", () => {
  it("links it to whoever is signed in in this tab, keeps their session, and forgets the value", async () => {
    arriveConnecting(`ticket=${TICKET}`);
    const connected = { data: undefined, error: undefined, response: new Response(null, { status: 204 }) } as never;
    const post = vi.spyOn(api, "POST").mockResolvedValue(connected);
    const onSignedIn = vi.fn();
    renderWithClient(<MicrosoftSignIn onSignedIn={onSignedIn} />);
    expect((await screen.findByText(/Your Microsoft account is connected/)).textContent).toContain("you can sign in to HiringPlug with it");
    expect(post).toHaveBeenCalledTimes(1);
    expect(post.mock.calls[0]?.[0]).toBe("/v1/me/microsoft/complete");
    expect(sentBody(post)).toEqual({ ticket: TICKET, binding: VALUE });
    expect(getToken()).toBe("tok_signed_in");
    expect(onSignedIn).not.toHaveBeenCalled();
    expect(takeBinding()).toBeNull();
    expect(screen.getByRole("link", { name: "Back to Your account" }).getAttribute("href")).toBe("#/account");
    expect(window.location.href).not.toContain(TICKET);
  });

  it.each(Object.keys(NOT_CONNECTED))("says what %s means", async (reason) => {
    arriveConnecting(`ticket=${TICKET}`);
    vi.spyOn(api, "POST").mockResolvedValue(failure("MICROSOFT_CONNECT_FAILED", "server words", { reason }, 422));
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toBe(NOT_CONNECTED[reason]);
    expect(screen.getByRole("link", { name: "Back to Your account" })).toBeTruthy();
    expect(takeBinding()).toBeNull();
  });

  it("says when it was cancelled, without asking the server", async () => {
    arriveConnecting("error=cancelled");
    const post = vi.spyOn(api, "POST");
    renderWithClient(<MicrosoftSignIn onSignedIn={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toContain("You cancelled");
    expect(screen.getByRole("heading", { name: "Connect Microsoft sign-in" })).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });
});

it("tells an owner to connect their account first", () => {
  expect(REFUSED.connect_first).toContain("Your account › Connect Microsoft sign-in");
});
