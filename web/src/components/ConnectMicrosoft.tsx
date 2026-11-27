import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Schemas, unwrap } from "../api/client";
import { startConnecting } from "../microsoft";
import { ErrorMessage } from "./ErrorMessage";
import { fieldErrors } from "./forms";
import { MicrosoftMark } from "./MicrosoftButton";
import "./sign-in.css";

/**
 * Your account › Microsoft sign-in (when the company signs in with
 * Microsoft): connect your Microsoft work account, signed in. Owners do it
 * here: a Microsoft account is never connected to an owner by its address.
 * It asks for your password (and two-step code) first: a session left open
 * on someone's computer is not enough.
 */
export function ConnectMicrosoft({ me }: { me: Schemas["Me"] }) {
  const [asking, setAsking] = useState(false);
  if (!me.microsoftSignIn) return null;
  return (
    <section className="panel ms-connect" aria-label="Microsoft sign-in">
      <h3><MicrosoftMark /> Microsoft sign-in</h3>
      {me.microsoftLinked ? (
        <p><span className="badge accepted">Connected</span> You can sign in with your Microsoft work account. To use another one, ask an owner to unlink this one on the Team page.</p>
      ) : (
        <>
          <p>
            Connect your Microsoft work account to sign in to HyPlug with it.
            {me.role === "owner" && " As an owner, you connect it here, once: an account is never connected to an owner just because its email matches."}
          </p>
          {asking ? <Prove hasPassword={me.hasPassword} onCancel={() => { setAsking(false); }} /> : (
            <div className="row">
              <button type="button" className="ms-button" onClick={() => { setAsking(true); }}>
                <MicrosoftMark /> Connect Microsoft sign-in
              </button>
            </div>
          )}
        </>
      )}
    </section>
  );
}

/** Your password (and two-step code, if you use it), then off to Microsoft. */
function Prove({ hasPassword, onCancel }: { hasPassword: boolean; onCancel: () => void }) {
  const twoStep = useQuery({ queryKey: ["me", "two-step"], queryFn: async () => unwrap(await api.GET("/v1/me/two-step")) });
  const [form, setForm] = useState({ password: "", code: "" });
  const start = useMutation({ mutationFn: () => startConnecting(form) });
  // Once started the browser is leaving for Microsoft: the button stays busy.
  const busy = start.isPending || start.isSuccess;
  const needsCode = twoStep.data?.enabled === true;
  const fields = fieldErrors(start.error);
  const reason = start.error instanceof ApiError && start.error.code === "MICROSOFT_CONNECT_FAILED" ? start.error.details.reason : "";
  return (
    <form className="ms-prove" aria-label="Confirm it is you" onSubmit={(e) => { e.preventDefault(); start.mutate(); }}>
      <p className="muted small full">First confirm it is you{hasPassword ? "" : needsCode ? " with your authenticator app" : ""}.</p>
      {hasPassword && (
        <label className="field">Your current password
          <input type="password" required autoComplete="current-password" value={form.password} aria-invalid={!!fields.password}
            onChange={(e) => { setForm({ ...form, password: e.target.value }); }} />
          {fields.password && <span className="field-error" role="alert">Password {fields.password}</span>}
        </label>
      )}
      {needsCode && (
        <label className="field">Two-step code (or a recovery code)
          <input required inputMode="numeric" autoComplete="one-time-code" maxLength={20} value={form.code} aria-invalid={!!fields.code}
            onChange={(e) => { setForm({ ...form, code: e.target.value }); }} />
          {fields.code && <span className="field-error" role="alert">Code: {fields.code}</span>}
        </label>
      )}
      <div className="row full">
        <button type="submit" className="ms-button" disabled={busy || twoStep.isPending} aria-busy={busy}>
          <MicrosoftMark /> {busy ? "Opening Microsoft…" : "Continue to Microsoft"}
        </button>
        <button type="button" className="link" disabled={busy} onClick={onCancel}>Cancel</button>
      </div>
      {reason === "already_connected"
        ? <p className="muted small full" role="status">A Microsoft account is already connected to you.</p>
        : Object.keys(fields).length === 0 && <div className="full"><ErrorMessage error={start.error} /></div>}
    </form>
  );
}
