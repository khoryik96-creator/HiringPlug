// Development sign-in: a token from `hp dev-setup`, kept for this browser tab.
// Production sign-in (Google / Microsoft single sign-on) replaces this module.
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
