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

/** The invitation, password reset or Microsoft reconnect link in the address (#/invite/<token>, #/reset/<token>, #/reconnect/<token>), if any. */
export function authLinkRoute(hash: string): { kind: "invite" | "reset" | "reconnect"; token: string } | null {
  const m = /^#\/(invite|reset|reconnect)\/([0-9a-f]{64})$/.exec(hash);
  const [, kind, token] = m ?? [];
  return kind && token ? { kind: kind as "invite" | "reset" | "reconnect", token } : null;
}

// A new browser tab of HyPlug (a record's tab dragged out, a link opened
// in a new tab) takes the sign-in of a tab already signed in, asked over this
// website's own channel (no other website can hear it); with none open, it
// asks to sign in as before. Closing every tab still signs out.
const CHANNEL = "hiringplug.session";
type Message = { kind: "ask" } | { kind: "token"; token: string };

/** Answers other new tabs while this one is signed in; returns the stop. */
export function shareSignIn(): () => void {
  if (typeof BroadcastChannel !== "function") return () => {};
  const channel = new BroadcastChannel(CHANNEL);
  channel.onmessage = (e: MessageEvent<Message>) => {
    const token = getToken();
    if (e.data.kind === "ask" && token) channel.postMessage({ kind: "token", token } satisfies Message);
  };
  return () => { channel.close(); };
}

/** Asks the tabs already signed in for their sign-in; null when none
 * answers in time, or when tabs signed in as different people answer (the
 * new tab cannot tell whose it should be: it asks to sign in). */
export function askSignIn(waitMs = 500): Promise<string | null> {
  if (typeof BroadcastChannel !== "function") return Promise.resolve(null);
  return new Promise((done) => {
    const channel = new BroadcastChannel(CHANNEL);
    const answers = new Set<string>();
    window.setTimeout(() => {
      channel.close();
      done(answers.size === 1 ? [...answers][0] ?? null : null);
    }, waitMs);
    channel.onmessage = (e: MessageEvent<Message>) => {
      if (e.data.kind === "token" && typeof e.data.token === "string" && e.data.token) answers.add(e.data.token);
    };
    channel.postMessage({ kind: "ask" } satisfies Message);
  });
}
