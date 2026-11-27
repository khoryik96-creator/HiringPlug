// What the browser tab says when Apollo sends someone back to /callback. A
// failure names the step that failed, with Apollo's status or the scopes it
// granted (never a code, token or secret), so whoever runs the broker can tell
// which setting to fix instead of guessing.

export class SignInFailure extends Error {
  constructor(step, detail = "") {
    super(step);
    this.step = step;
    this.detail = detail;
  }
}

const WORD = /^[a-z0-9_:.-]{1,64}$/i;
const words = (text, limit) =>
  typeof text === "string"
    ? text
        .split(/[\s,]+/)
        .filter((w) => WORD.test(w))
        .slice(0, limit)
        .join(", ")
    : "";

// The scopes Apollo granted, as plain words.
export const grantedScopes = (scope) => words(scope, 20);
// Apollo's error code from the redirect (for example access_denied).
export const apolloError = (error) => words(error, 1);

const AGAIN = "Return to the app where you started and connect again.";

export function signInMessage(failure) {
  const detail = failure?.detail ?? "";
  switch (failure?.step) {
    case "connected":
      return "Apollo connected. Return to the app where you started. You can close this tab.";
    case "declined":
      return `Apollo sign-in was declined${detail ? ` (Apollo said: ${detail})` : ""}. ${AGAIN}`;
    case "finishing":
      return "Apollo sign-in is still finishing. Return to the app where you started and click Check connection in a few seconds.";
    case "expired":
      return `This sign-in link has expired or was already used. ${AGAIN}`;
    case "credentials":
      return (
        `Apollo refused this broker's app details (Apollo answered ${detail || "an error"}). ` +
        "The broker's APOLLO_CLIENT_ID and APOLLO_CLIENT_SECRET must be the Client ID and Client Secret " +
        "of the Apollo app whose redirect URL is this broker's /callback."
      );
    case "scope":
      return (
        "Apollo signed you in but did not grant email lookup. " +
        `Granted: ${detail || "none"}. Needed: read_user_profile and people_match. ` +
        "Add the missing scopes to the Apollo app, then connect again."
      );
    case "token":
      return `Apollo's sign-in answer was incomplete (no refresh token or expiry). ${AGAIN}`;
    case "profile":
      return `Apollo did not return your profile (Apollo answered ${detail || "an error"}). ${AGAIN}`;
    default:
      return "Apollo sign-in was interrupted. Return to the app where you started and check your account permissions before connecting again.";
  }
}
