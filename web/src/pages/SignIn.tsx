import { useMutation } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, unwrap } from "../api/client";
import { setToken } from "../auth";
import { ErrorMessage } from "../components/ErrorMessage";

type Company = { slug: string; name: string };

/** Sign in with email and password; or ask for a password reset link. */
export function SignIn({ onSignedIn }: { onSignedIn: () => void }) {
  const [mode, setMode] = useState<"sign-in" | "forgot" | "sent">("sign-in");
  const [form, setForm] = useState({ email: "", password: "", company: "" });
  const signIn = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/auth/sign-in", {
      body: { email: form.email.trim(), password: form.password, ...(form.company ? { company: form.company } : {}) },
    })),
    onSuccess: (s) => { setToken(s.token); onSignedIn(); },
  });
  const reset = useMutation({
    mutationFn: async () => {
      const res = await api.POST("/v1/auth/password-resets", { body: { email: form.email.trim() } });
      if (!res.response.ok) unwrap(res); // 202 has no body: only an error needs reading
    },
    onSuccess: () => { setMode("sent"); },
  });
  const companies = signIn.error instanceof ApiError && signIn.error.code === "SIGN_IN_CHOOSE_COMPANY"
    ? (signIn.error.details.companies as Company[]) : null;

  if (mode === "sent") {
    return (
      <main className="narrow">
        <h1>HiringPlug</h1>
        <p role="status">If {form.email} belongs to an account, a link to choose a new password is on its way. It works for one hour.</p>
        <button type="button" className="link" onClick={() => { setMode("sign-in"); }}>Back to sign in</button>
      </main>
    );
  }
  if (mode === "forgot") {
    return (
      <main className="narrow">
        <h1>HiringPlug</h1>
        <form className="form-grid" aria-label="Reset password" onSubmit={(e) => { e.preventDefault(); reset.mutate(); }}>
          <p className="full">We will email you a link to choose a new password.</p>
          <label className="field full">Email<input type="email" autoComplete="email" value={form.email} onChange={(e) => { setForm({ ...form, email: e.target.value }); }} /></label>
          <ErrorMessage error={reset.error} />
          <div className="row full">
            <button type="submit" disabled={reset.isPending || !form.email.trim()}>Send link</button>
            <button type="button" className="link" onClick={() => { setMode("sign-in"); }}>Cancel</button>
          </div>
        </form>
      </main>
    );
  }
  return (
    <main className="narrow">
      <h1>HiringPlug</h1>
      <form className="form-grid" aria-label="Sign in" onSubmit={(e) => { e.preventDefault(); signIn.mutate(); }}>
        <label className="field full">Email
          <input type="email" autoComplete="username" value={form.email} onChange={(e) => { setForm({ ...form, email: e.target.value, company: "" }); }} />
        </label>
        <label className="field full">Password
          <input type="password" autoComplete="current-password" value={form.password} onChange={(e) => { setForm({ ...form, password: e.target.value }); }} />
        </label>
        {companies && (
          <fieldset className="full">
            <legend>Your account is at several companies. Which one?</legend>
            {companies.map((c) => (
              <label key={c.slug} className="row">
                <input type="radio" name="company" checked={form.company === c.slug} onChange={() => { setForm({ ...form, company: c.slug }); }} /> {c.name}
              </label>
            ))}
          </fieldset>
        )}
        {!companies && <ErrorMessage error={signIn.error} />}
        <div className="row full">
          <button type="submit" disabled={signIn.isPending || !form.email.trim() || !form.password || (companies !== null && !form.company)}>Sign in</button>
          <button type="button" className="link" onClick={() => { setMode("forgot"); }}>Forgot your password?</button>
        </div>
      </form>
      <DeveloperToken onSignedIn={onSignedIn} />
    </main>
  );
}

/** Developers sign in with a token from `hp dev-setup` or `hp token`. */
function DeveloperToken({ onSignedIn }: { onSignedIn: () => void }) {
  const [token, setValue] = useState("");
  return (
    <details className="muted">
      <summary>Sign in with a token (developers)</summary>
      <form onSubmit={(e) => { e.preventDefault(); setToken(token); onSignedIn(); }}>
        <textarea aria-label="Token" rows={3} value={token} onChange={(e) => { setValue(e.target.value); }} />
        <button type="submit" disabled={!token.trim()}>Use token</button>
      </form>
    </details>
  );
}
