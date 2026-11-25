import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, unwrap } from "../api/client";
import { setToken } from "../auth";
import { ErrorMessage } from "../components/ErrorMessage";
import { MicrosoftButton, OrDivider } from "../components/MicrosoftButton";
import { SaveError } from "../components/SaveError";
import { SignInFrame } from "../components/SignInFrame";
import { nextStepOf, TwoStepAtSignIn } from "../components/TwoStep";

/**
 * Choose a password with an invitation or password reset link, then sign
 * in; or connect a Microsoft account with a reconnect link (Microsoft only).
 */
export function ChoosePassword({ token, onSignedIn }: { token: string; onSignedIn: () => void }) {
  const link = useQuery({
    queryKey: ["auth-link", token],
    queryFn: async () => unwrap(await api.GET("/v1/auth/links/{linkToken}", { params: { path: { linkToken: token } } })),
  });
  const [form, setForm] = useState({ name: "", password: "", repeat: "" });
  const repeat = useRef<HTMLInputElement>(null);
  useEffect(() => { repeat.current?.setCustomValidity(form.repeat === form.password ? "" : "The two passwords differ."); }, [form.password, form.repeat]);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/auth/links/{linkToken}", {
      params: { path: { linkToken: token } },
      body: { password: form.password, ...(link.data?.kind === "invite" && form.name.trim() ? { name: form.name.trim() } : {}) },
    })),
    onSuccess: (s) => {
      setToken(s.token);
      window.location.hash = "#/";
      onSignedIn();
    },
  });
  const l = link.data;
  const next = nextStepOf(save.error); // the password is saved; two-step sign-in asks for its code
  const signedIn = () => { window.location.hash = "#/"; onSignedIn(); };
  if (next) {
    return (
      <SignInFrame>
        <p role="status">Password saved.</p>
        <TwoStepAtSignIn next={next} onSignedIn={signedIn} onCancel={() => { window.location.hash = "#/"; }} />
      </SignInFrame>
    );
  }
  // Microsoft instead of a password: offered, or (the company requires it) the only way.
  const microsoft = l?.microsoft ?? "none";
  const invite = l?.kind === "invite";
  if (l?.kind === "reconnect") {
    return (
      <SignInFrame>
        <h2 className="sign-in-title">Connect Microsoft sign-in</h2>
        {microsoft === "none" ? <p role="alert">{l.companyName} does not sign in with Microsoft any more. <a href="#/">Go to sign in</a></p> : (
          <>
            <p>Sign in with your Microsoft work account to connect it to your HiringPlug account at <strong>{l.companyName}</strong>. From then on it signs you in.</p>
            <p className="muted">{l.name} · {l.email}</p>
            <MicrosoftButton label="Connect with Microsoft" from={() => ({ invite: token })} />
            <p className="muted small">The link works once. If you did not expect it, close this page and tell an owner.</p>
          </>
        )}
      </SignInFrame>
    );
  }
  return (
    <SignInFrame>
      <ErrorMessage error={link.error} />
      {link.error && <a href="#/">Go to sign in</a>}
      {l && microsoft !== "none" && (
        <>
          <p>
            {invite ? <>Welcome to <strong>{l.companyName}</strong>. </> : null}
            {microsoft === "required"
              ? (invite ? `${l.companyName} signs in with Microsoft: accept with your work account.` : "Your company signs in with Microsoft: use Sign in with Microsoft.")
              : invite ? "Accept with your Microsoft work account, or choose a password." : `Choose a new password for ${l.companyName}, or sign in with Microsoft.`}
          </p>
          <p className="muted">{l.email}</p>
          <MicrosoftButton label={invite ? "Accept with Microsoft" : "Sign in with Microsoft"}
            from={() => (invite ? { invite: token } : { email: l.email })} />
          {microsoft === "required" && <p className="muted small"><a href="#/">Back to sign in</a></p>}
          {microsoft === "available" && <OrDivider>{invite ? "or choose a password" : "or choose a new password"}</OrDivider>}
        </>
      )}
      {l && microsoft !== "required" && (
        <form className="form-grid" aria-label="Choose your password" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
          {microsoft === "none" && <>
            <p className="full">
              {invite ? <>Welcome to <strong>{l.companyName}</strong>. Choose a password to join.</> : <>Choose a new password for {l.companyName}.</>}
            </p>
            <p className="full muted">{l.email}</p>
          </>}
          {invite && (
            <label className="field full">Your name<input value={form.name} placeholder={l.name} onChange={(e) => { setForm({ ...form, name: e.target.value }); }} /></label>
          )}
          <input type="email" autoComplete="username" value={l.email} readOnly hidden />
          <label className="field full">Password (at least 10 characters)
            <input type="password" required minLength={10} autoComplete="new-password" value={form.password} onChange={(e) => { setForm({ ...form, password: e.target.value }); }} />
          </label>
          <label className="field full">The same again
            <input ref={repeat} type="password" required autoComplete="new-password" value={form.repeat} onChange={(e) => { setForm({ ...form, repeat: e.target.value }); }} />
          </label>
          {form.repeat && form.repeat !== form.password && <p className="full warning" role="status">The two passwords differ.</p>}
          <SaveError error={save.error} shown={[]} onReload={() => undefined} />
          <div className="row full">
            <button type="submit" disabled={save.isPending}>
              {invite ? "Join" : "Save password"}
            </button>
          </div>
        </form>
      )}
    </SignInFrame>
  );
}
