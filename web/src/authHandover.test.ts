import { afterEach, expect, it } from "vitest";

import { askSignIn, clearToken, getToken, setToken, shareSignIn } from "./auth";

afterEach(() => { clearToken(); });

it("a new tab takes the sign-in of a tab already signed in, and none without one", async () => {
  expect(await askSignIn(100)).toBeNull(); // nobody signed in: sign in as before
  setToken("tok_signed_in");
  const stop = shareSignIn();
  try {
    expect(await askSignIn(500)).toBe("tok_signed_in");
    clearToken(); // that tab signed out: it no longer answers
    expect(getToken()).toBeNull();
    expect(await askSignIn(100)).toBeNull();
  } finally {
    stop();
  }
});

it("signs in nobody when tabs signed in as different people answer", async () => {
  setToken("tok_aina");
  const stop = shareSignIn();
  // Another tab of the browser, signed in as someone else.
  const other = new BroadcastChannel("hiringplug.session");
  other.onmessage = (e: MessageEvent<{ kind: string }>) => {
    if (e.data.kind === "ask") other.postMessage({ kind: "token", token: "tok_lim" });
  };
  try {
    expect(await askSignIn(300)).toBeNull(); // whose would it be? it asks to sign in
  } finally {
    stop();
    other.close();
  }
});
