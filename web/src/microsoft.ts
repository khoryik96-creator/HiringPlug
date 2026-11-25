// Sign in with Microsoft, the browser's part. Starting keeps a random value
// in this tab's sessionStorage and sends only its SHA-256; Microsoft sends
// the browser back to #/sign-in/microsoft?ticket=…, and finishing sends the
// value itself, so a ticket copied to another browser signs nobody in.
import { api, type Schemas, unwrap } from "./api/client";

const BINDING = "hiringplug.microsoft.binding";

/** The value this tab started a Microsoft sign-in with, if any. */
export function takeBinding(): string | null {
  try {
    return sessionStorage.getItem(BINDING);
  } catch {
    return null;
  }
}

export function clearBinding(): void {
  try {
    sessionStorage.removeItem(BINDING);
  } catch { /* nothing kept */ }
}

async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** A new random value for this tab (64 hex characters); returns its SHA-256. */
export async function newBinding(): Promise<string> {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  const value = [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
  sessionStorage.setItem(BINDING, value);
  return sha256Hex(value);
}

/** Where the browser goes next (replaced in tests: jsdom cannot navigate). */
export const browser = { go: (url: string) => { window.location.assign(url); } };

const looksLikeEmail = (s: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s);

type Start = Omit<Schemas["MicrosoftSignInStart"], "bindingHash">;

/** Starts signing in with Microsoft and sends the browser there. */
export async function startMicrosoft(from: Start): Promise<void> {
  if (typeof crypto === "undefined" || !("subtle" in crypto)) {
    throw new Error("Signing in with Microsoft needs the secure (https) address of HiringPlug.");
  }
  const email = from.email?.trim() ?? "";
  const body: Schemas["MicrosoftSignInStart"] = {
    bindingHash: await newBinding(),
    ...(looksLikeEmail(email) ? { email } : {}),
    ...(from.company ? { company: from.company } : {}),
    ...(from.invite ? { invite: from.invite } : {}),
  };
  const { url } = unwrap(await api.POST("/v1/auth/microsoft/start", { body }));
  browser.go(url);
}

/** The Microsoft sign-in page's address (#/sign-in/microsoft?ticket=… or ?error=…), if this is it. */
export function microsoftRoute(hash: string): { ticket: string; error: string } | null {
  const m = /^#\/sign-in\/microsoft(?:\?(.*))?$/.exec(hash);
  if (!m) return null;
  const q = new URLSearchParams(m[1] ?? "");
  return { ticket: q.get("ticket") ?? "", error: q.get("error") ?? "" };
}

/** Takes the ticket out of the address (and the browser's history). */
export function forgetTicket(to = "#/sign-in/microsoft"): void {
  window.history.replaceState(window.history.state, "", `${window.location.pathname}${window.location.search}${to}`);
}
