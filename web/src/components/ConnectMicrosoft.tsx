import { useMutation } from "@tanstack/react-query";

import { ApiError, type Schemas } from "../api/client";
import { startConnecting } from "../microsoft";
import { ErrorMessage } from "./ErrorMessage";
import { MicrosoftMark } from "./MicrosoftButton";
import "./sign-in.css";

/**
 * Your account › Microsoft sign-in (when the company signs in with
 * Microsoft): connect your Microsoft work account, signed in. Owners do it
 * here: a Microsoft account is never connected to an owner by its address.
 */
export function ConnectMicrosoft({ me }: { me: Schemas["Me"] }) {
  const start = useMutation({ mutationFn: startConnecting });
  if (!me.microsoftSignIn) return null;
  // Once started the browser is leaving for Microsoft: the button stays busy.
  const busy = start.isPending || start.isSuccess;
  const reason = start.error instanceof ApiError && start.error.code === "MICROSOFT_CONNECT_FAILED" ? start.error.details.reason : "";
  return (
    <section className="panel ms-connect" aria-label="Microsoft sign-in">
      <h3><MicrosoftMark /> Microsoft sign-in</h3>
      {me.microsoftLinked ? (
        <p><span className="badge accepted">Connected</span> You can sign in with your Microsoft work account. To use another one, ask an owner to unlink this one on the Team page.</p>
      ) : (
        <>
          <p>
            Connect your Microsoft work account to sign in to HiringPlug with it.
            {me.role === "owner" && " As an owner, you connect it here, once: an account is never connected to an owner just because its email matches."}
          </p>
          <div className="row">
            <button type="button" className="ms-button" disabled={busy} aria-busy={busy} onClick={() => { start.mutate(); }}>
              <MicrosoftMark /> {busy ? "Opening Microsoft…" : "Connect Microsoft sign-in"}
            </button>
          </div>
          {reason === "already_connected"
            ? <p className="muted small" role="status">A Microsoft account is already connected to you.</p>
            : <ErrorMessage error={start.error} />}
        </>
      )}
    </section>
  );
}
