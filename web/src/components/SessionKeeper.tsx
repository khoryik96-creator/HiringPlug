// Keeps a working person signed in, and warns an idle one before signing out.
// A sign-in lasts 12 hours. While someone is using the app it is renewed
// quietly (the same session, 12 hours from then); after they have been away,
// the last five minutes count down with "Stay signed in", then they are
// signed out.
import { useEffect, useRef, useState } from "react";

import { api, unwrap } from "../api/client";
import { getToken, setToken } from "../auth";

const MINUTE = 60_000;
/** Renewed quietly when this little is left, if they are working. */
export const RENEW_WITHIN = 30 * MINUTE;
/** Using the app within this long counts as working. */
export const ACTIVE_WITHIN = 10 * MINUTE;
/** The countdown before signing out. */
export const WARN_WITHIN = 5 * MINUTE;

/** When a sign-in token ends (ms), from its own claims; null when unreadable. */
export function tokenExpiry(token: string | null): number | null {
  const payload = token?.split(".")[1];
  if (!payload) return null;
  try {
    const json = atob(payload.replace(/-/g, "+").replace(/_/g, "/"));
    const exp: unknown = (JSON.parse(json) as { exp?: unknown }).exp;
    return typeof exp === "number" ? exp * 1000 : null;
  } catch {
    return null;
  }
}

const mmss = (ms: number) => {
  const s = Math.max(0, Math.ceil(ms / 1000));
  return `${String(Math.floor(s / 60))}:${String(s % 60).padStart(2, "0")}`;
};

export function SessionKeeper({ onSignOut }: { onSignOut: () => void }) {
  const lastActive = useRef(0); // set when the page starts watching
  const renewing = useRef(false);
  const lastTry = useRef(0); // a failed renewal is tried again a minute later, not every second
  const [left, setLeft] = useState<number | null>(null);
  const [failed, setFailed] = useState(false);

  const renew = async () => {
    if (renewing.current) return;
    renewing.current = true;
    lastTry.current = Date.now();
    try {
      const s = unwrap(await api.POST("/v1/auth/renew"));
      setToken(s.token);
      setFailed(false);
      const exp = tokenExpiry(s.token);
      setLeft(exp === null ? null : exp - Date.now());
    } catch {
      setFailed(true);
    } finally {
      renewing.current = false;
    }
  };

  useEffect(() => {
    const active = () => { lastActive.current = Date.now(); };
    active(); // opening the page counts as using it
    const events = ["pointerdown", "keydown", "wheel", "touchstart"] as const;
    for (const e of events) window.addEventListener(e, active, { passive: true });
    const tick = () => {
      const exp = tokenExpiry(getToken());
      if (exp === null) { setLeft(null); return; }
      const remaining = exp - Date.now();
      if (remaining <= 0) { onSignOut(); return; }
      const working = Date.now() - lastActive.current < ACTIVE_WITHIN;
      if (remaining < RENEW_WITHIN && working && remaining > WARN_WITHIN && Date.now() - lastTry.current > MINUTE) void renew();
      setLeft(remaining);
    };
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => {
      window.clearInterval(timer);
      for (const e of events) window.removeEventListener(e, active);
    };
    // renew and onSignOut read only refs and stable setters
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (left === null || left > WARN_WITHIN) return null;
  return (
    <div className="session-warning" role="alertdialog" aria-labelledby="session-warning-title" aria-describedby="session-warning-text">
      <strong id="session-warning-title">Still there?</strong>
      <span id="session-warning-text">You will be signed out in <b className="session-countdown">{mmss(left)}</b>.</span>
      {failed && <span className="warning small">Could not renew: sign in again to carry on.</span>}
      <div className="row">
        <button type="button" className="primary" autoFocus onClick={() => { void renew(); }}>Stay signed in</button>
        <button type="button" className="link" onClick={onSignOut}>Sign out</button>
      </div>
    </div>
  );
}
