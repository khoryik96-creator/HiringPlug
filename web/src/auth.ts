// The sign-in token, kept for this browser tab only (closing it signs out).
const KEY = "hiringplug.token";

export function getToken(): string | null {
  return sessionStorage.getItem(KEY);
}

export function setToken(token: string): void {
  sessionStorage.setItem(KEY, token.trim());
}

export function clearToken(): void {
  sessionStorage.removeItem(KEY);
}

/** The invitation or password reset link in the address (#/invite/<token>, #/reset/<token>), if any. */
export function authLinkRoute(hash: string): { kind: "invite" | "reset"; token: string } | null {
  const m = /^#\/(invite|reset)\/([0-9a-f]{64})$/.exec(hash);
  const [, kind, token] = m ?? [];
  return kind && token ? { kind: kind as "invite" | "reset", token } : null;
}
