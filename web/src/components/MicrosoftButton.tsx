import { useMutation } from "@tanstack/react-query";

import { ApiError, type Schemas } from "../api/client";
import { startMicrosoft } from "../microsoft";
import { ErrorMessage } from "./ErrorMessage";
import "./sign-in.css";

/** Microsoft's four squares, drawn here (no image is fetched). */
export function MicrosoftMark() {
  return (
    <svg className="ms-mark" aria-hidden="true" width="18" height="18" viewBox="0 0 21 21">
      <rect x="0" y="0" width="10" height="10" fill="#f25022" />
      <rect x="11" y="0" width="10" height="10" fill="#7fba00" />
      <rect x="0" y="11" width="10" height="10" fill="#00a4ef" />
      <rect x="11" y="11" width="10" height="10" fill="#ffb900" />
    </svg>
  );
}

/**
 * "Sign in with Microsoft": starts the sign-in and sends the browser to
 * Microsoft. `from` gives what is known so far (the email typed, the
 * invitation).
 */
export function MicrosoftButton({ label = "Sign in with Microsoft", from }: {
  label?: string;
  from: () => Omit<Schemas["MicrosoftSignInStart"], "bindingHash">;
}) {
  const start = useMutation({ mutationFn: async () => { await startMicrosoft(from()); } });
  const unavailable = start.error instanceof ApiError && start.error.code === "MICROSOFT_SIGN_IN_UNAVAILABLE";
  // Once started the browser is leaving for Microsoft: the button stays busy.
  const busy = start.isPending || start.isSuccess;
  return (
    <div className="ms-start">
      <button type="button" className="ms-button" disabled={busy} aria-busy={busy} onClick={() => { start.mutate(); }}>
        <MicrosoftMark /> {busy ? "Opening Microsoft…" : label}
      </button>
      {unavailable
        ? <p className="muted small ms-note" role="status">Signing in with Microsoft isn&apos;t set up for your company yet. Sign in with your email and password, or ask an owner.</p>
        : <ErrorMessage error={start.error} />}
    </div>
  );
}

/** The line between Microsoft and the email form: "or with your email". */
export function OrDivider({ children = "or with your email" }: { children?: string }) {
  return <p className="ms-or" role="separator" aria-label={children}><span>{children}</span></p>;
}
