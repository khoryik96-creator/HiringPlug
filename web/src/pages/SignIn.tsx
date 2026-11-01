import { useState } from "react";

import { setToken } from "../auth";

/** Development sign-in: paste a token printed by `hp dev-setup`. */
export function SignIn({ onSignedIn }: { onSignedIn: () => void }) {
  const [token, setValue] = useState("");
  return (
    <main className="narrow">
      <h1>HiringPlug</h1>
      <p className="muted">Paste a development token from <code>hp dev-setup</code>. Production uses single sign-on.</p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setToken(token);
          onSignedIn();
        }}
      >
        <textarea aria-label="Token" rows={4} value={token} onChange={(e) => { setValue(e.target.value); }} />
        <button type="submit" disabled={!token.trim()}>
          Sign in
        </button>
      </form>
    </main>
  );
}
