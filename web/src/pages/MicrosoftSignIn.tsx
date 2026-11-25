import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { api, ApiError, unwrap } from "../api/client";
import { setToken } from "../auth";
import { ErrorMessage } from "../components/ErrorMessage";
import { SignInFrame } from "../components/SignInFrame";
import { clearBinding, forgetTicket, isConnecting, microsoftRoute, takeBinding } from "../microsoft";

type Company = { slug: string; name: string };

/** What each refusal (MICROSOFT_SIGN_IN_FAILED's details.reason) means for the person. */
export const REFUSED: Record<string, string> = {
  not_on_team: "Your Microsoft account isn't on this HiringPlug team yet. Ask an owner to invite you.",
  other_organisation: "This Microsoft account belongs to another organisation. Sign in with your work account at this company.",
  personal_account: "This is a personal Microsoft account. Sign in with your work account instead.",
  connect_first: "This Microsoft account has to be connected to your HiringPlug account on purpose first: sign in with your password and use Your account › Connect Microsoft sign-in, or ask an owner to send you a link to connect it.",
  linked_elsewhere: "This Microsoft account is already connected to someone else on the team. Use your own work account.",
  already_connected: "Another Microsoft account is already connected to you. Ask an owner to unlink it on the Team page first.",
  failed: "Signing in with Microsoft didn't work, or took too long. Please start again.",
};

/** What each refusal to connect an account (MICROSOFT_CONNECT_FAILED's details.reason) means. */
export const NOT_CONNECTED: Record<string, string> = {
  already_connected: "A Microsoft account is already connected to you. To use another, ask an owner to unlink it on the Team page first.",
  linked_elsewhere: "This Microsoft account is already connected to someone else on your team. Choose your own account.",
  other_organisation: "This Microsoft account belongs to another organisation (or is a guest in yours). Use your work account at this company.",
  personal_account: "This is a personal Microsoft account. Use your work account instead.",
  failed: "Connecting your Microsoft account didn't work, or took too long. Please start again from Your account.",
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
  const [arrived] = useState(() => ({ ...(microsoftRoute(window.location.hash) ?? { ticket: "", error: "" }), binding: takeBinding(), connecting: isConnecting() }));
  if (arrived.connecting) return <FinishConnecting arrived={arrived} />;
  return <FinishSignIn arrived={arrived} onSignedIn={onSignedIn} />;
}

type Arrived = { ticket: string; error: string; binding: string | null };

/** Why there is nothing to finish (a key of NOTHING_TO_FINISH), or "". */
const nothingToFinish = (a: Arrived) => a.error ? (a.error === "cancelled" ? "cancelled" : "failed")
  : !a.ticket ? "ticket" : !a.binding ? "binding" : "";

/** Finishes (once) as soon as the page opens, unless there is nothing to finish. */
function useFinishOnce(arrived: Arrived, finish: (ticket: string) => void) {
  const why = nothingToFinish(arrived);
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return; // once, even when React runs effects twice
    started.current = true;
    forgetTicket();
    if (why) clearBinding();
    else finish(arrived.ticket);
  }, [arrived, finish, why]);
  return why;
}

/**
 * Back from connecting a Microsoft account (Your account, signed in): links
 * it to the person signed in in this tab, or says why it did not work.
 */
function FinishConnecting({ arrived }: { arrived: Arrived }) {
  const cache = useQueryClient();
  const connect = useMutation({
    mutationFn: async (ticket: string) => {
      unwrap(await api.POST("/v1/me/microsoft/complete", { body: { ticket, binding: arrived.binding ?? "" } }));
    },
    onSettled: async () => { clearBinding(); await cache.invalidateQueries({ queryKey: ["me"] }); },
  });
  const why = useFinishOnce(arrived, connect.mutate);
  const err = connect.error;
  const reason = err instanceof ApiError && err.code === "MICROSOFT_CONNECT_FAILED"
    ? (typeof err.details.reason === "string" ? err.details.reason : "failed") : "";
  const back = <a href="#/account" className="button">Back to Your account</a>;
  return (
    <SignInFrame>
      <h2 className="sign-in-title">Connect Microsoft sign-in</h2>
      {why ? (
        <div className="ms-outcome"><p role="alert">{NOTHING_TO_FINISH[why]}</p>{back}</div>
      ) : connect.isSuccess ? (
        <div className="ms-outcome"><p role="status">Your Microsoft account is connected. From now on you can sign in to HiringPlug with it.</p>{back}</div>
      ) : err ? (
        <div className="ms-outcome">{reason ? <p role="alert">{NOT_CONNECTED[reason] ?? NOT_CONNECTED.failed}</p> : <ErrorMessage error={err} />}{back}</div>
      ) : (
        <p className="muted" role="status">Connecting your Microsoft account…</p>
      )}
    </SignInFrame>
  );
}

function FinishSignIn({ arrived, onSignedIn }: { arrived: Arrived; onSignedIn: () => void }) {
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
  const { mutate } = complete;
  const finish = useCallback((ticket: string) => { mutate({ ticket }); }, [mutate]);
  const why = useFinishOnce(arrived, finish);

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
