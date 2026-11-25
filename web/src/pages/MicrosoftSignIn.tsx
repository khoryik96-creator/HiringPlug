import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { api, ApiError, unwrap } from "../api/client";
import { setToken } from "../auth";
import { ErrorMessage } from "../components/ErrorMessage";
import { SignInFrame } from "../components/SignInFrame";
import { clearBinding, forgetTicket, microsoftRoute, takeBinding } from "../microsoft";

type Company = { slug: string; name: string };

/** What each refusal (MICROSOFT_SIGN_IN_FAILED's details.reason) means for the person. */
export const REFUSED: Record<string, string> = {
  not_on_team: "Your Microsoft account isn't on this HiringPlug team yet. Ask an owner to invite you.",
  other_organisation: "This Microsoft account belongs to another organisation. Sign in with your work account at this company.",
  personal_account: "This is a personal Microsoft account. Sign in with your work account instead.",
  failed: "Signing in with Microsoft didn't work, or took too long. Please start again.",
};

/** Why there is nothing to finish, before asking the server. */
const NOTHING_TO_FINISH: Record<string, string> = {
  cancelled: "You cancelled signing in with Microsoft. Nothing has changed.",
  failed: "Microsoft didn't finish signing you in. Please start again.",
  binding: "This sign-in was started in another browser or tab, so it can't finish here. Please start again in this one.",
  ticket: "This sign-in has already finished or has expired. Please start again.",
};

/**
 * Where Microsoft's sign-in comes back to (#/sign-in/microsoft?ticket=…):
 * finishes it in the tab that started it, asks which company when the
 * account is at several, or says why it did not work.
 */
export function MicrosoftSignIn({ onSignedIn }: { onSignedIn: () => void }) {
  // Read once: the address loses its ticket at once (history included).
  const [arrived] = useState(() => ({ ...(microsoftRoute(window.location.hash) ?? { ticket: "", error: "" }), binding: takeBinding() }));
  const [choice, setChoice] = useState<{ ticket: string; companies: Company[] } | null>(null);
  const complete = useMutation({
    mutationFn: async (to: { ticket: string; company?: string }) => unwrap(await api.POST("/v1/auth/microsoft/complete", {
      body: { ticket: to.ticket, binding: arrived.binding ?? "", ...(to.company ? { company: to.company } : {}) },
    })),
    onSuccess: (s) => {
      clearBinding();
      setToken(s.token);
      forgetTicket("#/");
      onSignedIn();
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === "SIGN_IN_CHOOSE_COMPANY" && typeof e.details.ticket === "string") {
        setChoice({ ticket: e.details.ticket, companies: Array.isArray(e.details.companies) ? e.details.companies as Company[] : [] });
        return; // the same tab finishes with the company chosen
      }
      clearBinding();
    },
  });
  const why = arrived.error ? (arrived.error === "cancelled" ? "cancelled" : "failed")
    : !arrived.ticket ? "ticket" : !arrived.binding ? "binding" : "";
  const started = useRef(false);
  const { mutate } = complete;
  useEffect(() => {
    if (started.current) return; // once, even when React runs effects twice
    started.current = true;
    forgetTicket();
    if (why) clearBinding();
    else mutate({ ticket: arrived.ticket });
  }, [arrived, mutate, why]);

  const err = complete.error;
  const reason = err instanceof ApiError && err.code === "MICROSOFT_SIGN_IN_FAILED"
    ? (typeof err.details.reason === "string" ? err.details.reason : "failed") : "";
  const back = <a href="#/" className="button">Back to sign in</a>;
  if (choice && !complete.isSuccess) {
    return (
      <SignInFrame>
        <h2 className="sign-in-title">Your Microsoft account is at several companies. Which one?</h2>
        <ul className="ms-companies" aria-label="Companies">
          {choice.companies.map((c) => (
            <li key={c.slug}>
              <button type="button" disabled={complete.isPending} onClick={() => { complete.mutate({ ticket: choice.ticket, company: c.slug }); }}>{c.name}</button>
            </li>
          ))}
        </ul>
        {complete.isPending && <p className="muted" role="status">Signing you in…</p>}
        {err && !(err instanceof ApiError && err.code === "SIGN_IN_CHOOSE_COMPANY") && <Outcome reason={reason} error={err} back={back} />}
      </SignInFrame>
    );
  }
  return (
    <SignInFrame>
      <h2 className="sign-in-title">Sign in with Microsoft</h2>
      {why ? (
        <div className="ms-outcome"><p role="alert">{NOTHING_TO_FINISH[why]}</p>{back}</div>
      ) : err ? (
        <Outcome reason={reason} error={err} back={back} />
      ) : (
        <p className="muted" role="status">Signing you in…</p>
      )}
    </SignInFrame>
  );
}

function Outcome({ reason, error, back }: { reason: string; error: unknown; back: ReactNode }) {
  return (
    <div className="ms-outcome">
      {reason ? <p role="alert">{REFUSED[reason] ?? REFUSED.failed}</p> : <ErrorMessage error={error} />}
      {back}
    </div>
  );
}
