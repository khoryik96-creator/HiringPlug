import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useLayoutEffect, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";

type Connection = Schemas["ApolloConnection"];
type ConnectionAction = Schemas["ApolloConnectionAction"]["action"];
type Lookup = Schemas["ApolloLookup"];
type LookupAction = Schemas["ApolloLookupRequest"]["action"];

const connectionKey = ["hr-apollo-connection"] as const;
export function useHRApollo(enabled: boolean) {
  return useQuery({
    queryKey: connectionKey, enabled,
    queryFn: async ({ signal }) => unwrap(await api.GET("/v1/contacts/apollo", { signal })),
  });
}

function ApolloDialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const previous = useRef<Element | null>(null);
  useLayoutEffect(() => {
    const node = dialog.current;
    previous.current = document.activeElement;
    node?.showModal();
    return () => {
      node?.close();
      if (previous.current instanceof HTMLElement) previous.current.focus();
    };
  }, []);
  return <dialog ref={dialog} className="panel hr-apollo-dialog" aria-labelledby="hr-apollo-title"
    onCancel={(e) => { e.preventDefault(); onClose(); }}>
    <div className="row spread">
      <h3 id="hr-apollo-title">{title}</h3>
      <button type="button" className="link" onClick={onClose}>Close</button>
    </div>
    {children}
  </dialog>;
}

export function ApolloConnectionDialog({ connection, onClose }: { connection: ReturnType<typeof useHRApollo>; onClose: () => void }) {
  const cache = useQueryClient();
  const change = useMutation({
    mutationFn: async (action: ConnectionAction) => unwrap(await api.POST("/v1/contacts/apollo", { body: { action } })),
    onSuccess: (next) => { cache.setQueryData(connectionKey, next); },
  });
  const current: Connection | undefined = connection.data;
  const state = current?.state;
  return <ApolloDialog title="Apollo connection" onClose={onClose}>
    <ErrorMessage error={change.error ?? connection.error} />
    {connection.isPending && <p role="status">Checking connection…</p>}
    {state === "disabled" && <p>Ask your HyPlug administrator to configure the Apollo sign-in broker.</p>}
    {state === "connected" && <p>Connected as <strong>{current?.accountName}</strong>.</p>}
    {state === "expired" && <p>Your Apollo sign-in has expired. Disconnect it, then connect again.</p>}
    {state === "failed" && <p>Apollo didn’t finish sign-in. Cancel this attempt, then connect again.</p>}
    {state === "disconnected_locally" && <p>HyPlug stopped lookups. Retry disconnecting to end access at the broker.</p>}
    {state === "pending" && <p>Open Apollo to sign in, then return here to check your connection.</p>}
    <div className="row hr-apollo-actions">
      {state === "pending" && current?.authorizationUrl &&
        <a href={current.authorizationUrl} target="_blank" rel="noopener noreferrer">Open Apollo ↗</a>}
      {state === "pending" && <button type="button" className="primary" disabled={change.isPending}
        onClick={() => { change.mutate("poll"); }}>{change.isPending ? "Checking…" : "Check connection"}</button>}
      {(state === "connected" || state === "pending" || state === "expired" || state === "failed" || state === "disconnected_locally") && <button type="button" disabled={change.isPending} onClick={() => { change.mutate("disconnect"); }}>
        {state === "pending" || state === "failed" ? "Cancel sign-in" : state === "disconnected_locally" ? "Retry disconnect" : "Disconnect"}
      </button>}
      {state === "disconnected" &&
        <button type="button" className="primary" disabled={change.isPending} onClick={() => { change.mutate("start"); }}>
          {change.isPending ? "Starting…" : "Connect Apollo"}
        </button>}
    </div>
    {state === "disconnected_locally" && <p><button type="button" className="link" disabled={change.isPending}
      onClick={() => { change.mutate("forget"); }}>I revoked access in Apollo. Forget this connection.</button></p>}
  </ApolloDialog>;
}

const issues: Partial<Record<Lookup["state"], string>> = {
  ineligible: "Apollo lookup is available for people who may still work at this company.",
  rescan: "Run a new Find HR search to verify this company’s LinkedIn page before using Apollo.",
  disabled: "Ask your HyPlug administrator to configure Apollo.",
  disconnected: "Connect your own Apollo account first.",
  previous_connection: "This person already had a lookup on another connection. Check Apollo for the result and any credit use.",
  busy: "Another lookup is in progress on this Apollo connection.",
  queued: "Queued. You can close this window and come back to check.",
  sending: "Checking Apollo…",
  mismatch: "Apollo couldn’t verify the LinkedIn profile and employer together.",
  pending: "Apollo is still processing this lookup. Check again for the result.",
  unconfirmed: "Apollo couldn’t confirm the result. A credit may have been used.",
  auth_required: "Apollo needs you to disconnect and reconnect. Check Apollo for any credit use on this lookup.",
  limited: "Apollo limited the lookup. Check your plan, credits, and account activity in Apollo.",
  cancelled: "The company, contact, or connection changed before the lookup. No Apollo request was sent.",
};

const absent: Record<NonNullable<Lookup["reason"]>, string> = {
  profile_not_found: "Apollo didn’t find this profile.",
  email_not_returned: "Apollo didn’t return a work email.",
  personal_email_only: "Apollo returned a personal address only.",
  email_redacted: "Apollo kept the email hidden.",
  email_unusable: "Apollo returned an unusable address.",
  contact_unconfirmed: "Apollo couldn’t confirm that the saved address belongs to this person at this company.",
  queue_expired: "The lookup expired in the queue.",
  retryable_limit: "The broker was busy before it could start this lookup.",
  reconnect_required: "Disconnect and reconnect Apollo to try again.",
  no_credits: "Apollo says your account has no credits left for lookups through apps. Check your credits in Apollo.",
  plan_not_allowed: "Apollo refused this lookup for your account: its plan may not allow email lookups through other apps, even when the Apollo LinkedIn extension works. Check your plan in Apollo.",
  rate_limited: "Apollo says too many lookups were made in a short time. Try again in a few minutes.",
};

const statusLabel: Record<NonNullable<Lookup["emailStatus"]>, string> = {
  verified: "Verified by Apollo", unverified: "Unverified", catch_all: "Catch-all domain", unavailable: "Unavailable",
  user_managed: "User-managed", unknown: "Verification unknown",
};

export function ApolloLookupDialog({ companyId, companyName, person, onClose, openConnection }: {
  companyId: string; companyName: string; person: Schemas["HrScanPerson"]; onClose: () => void; openConnection: () => void;
}) {
  const cache = useQueryClient();
  const key = ["hr-apollo-lookup", companyId, person.url] as const;
  const send = async (action: LookupAction, confirmation?: string, signal?: AbortSignal) => unwrap(
    await api.POST("/v1/companies/{companyId}/hr-scan/apollo", {
      params: { path: { companyId } }, body: { profileUrl: person.url, action, confirmation }, signal,
    }),
  );
  const [polling, setPolling] = useState(true);
  const [confirmed, setConfirmed] = useState(0); // each confirmation is watched for its own 75 seconds
  const lookup = useQuery({
    queryKey: key, queryFn: ({ signal }) => send("preview", undefined, signal), gcTime: 0,
    refetchInterval: (query) => polling && (query.state.data?.state === "queued" || query.state.data?.state === "sending") ? 2500 : false,
  });
  useEffect(() => {
    const stop = window.setTimeout(() => { setPolling(false); }, 75_000);
    return () => { window.clearTimeout(stop); };
  }, [confirmed]);
  const change = useMutation({
    mutationFn: ({ action, confirmation }: { action: LookupAction; confirmation?: string }) => send(action, confirmation),
    onMutate: async () => { await cache.cancelQueries({ queryKey: key }); },
    onSuccess: (next, { action }) => {
      cache.setQueryData(key, next);
      if (action === "unlock") {
        setPolling(true);
        setConfirmed((n) => n + 1);
      } else if ((next.state === "cancelled" || next.state === "ready") && !next.confirmation && next.reason !== "reconnect_required") {
        // A status check found the lookup never reached Apollo: preview again so it can be confirmed.
        void cache.invalidateQueries({ queryKey: key });
      }
    },
  });
  const result = lookup.data;
  return <ApolloDialog title="Work email" onClose={onClose}>
    <p className="hr-apollo-person"><strong>{person.name}</strong> · {companyName}</p>
    <ErrorMessage error={change.error ?? lookup.error} />
    {lookup.isPending && <p role="status">Checking HyPlug…</p>}
    {result && <div aria-live="polite">
      {result.email && <p className="hr-apollo-address"><strong>{result.email}</strong><br />
        <span className="muted">{result.state === "existing" ? "Already in HyPlug. No Apollo credit used." : statusLabel[result.emailStatus ?? "unknown"]}</span></p>}
      {issues[result.state] && <p>{(result.state === "cancelled" || result.state === "limited") && result.reason ? absent[result.reason] : issues[result.state]}</p>}
      {result.state === "not_found" && <p>{absent[result.reason ?? "email_not_returned"]} Check Apollo for any credit use.</p>}
      {(result.state === "ready" || result.state === "cancelled") && result.confirmation && <>
        <p>This lookup may use a credit from your Apollo account, even if no email is returned.</p>
        <button type="button" className="primary" disabled={change.isPending}
          onClick={() => { change.mutate({ action: "unlock", confirmation: result.confirmation }); }}>
          {change.isPending ? "Confirming…" : "Confirm credit use"}
        </button>
      </>}
      {(result.state === "unconfirmed" || result.state === "pending") && <button type="button" disabled={change.isPending}
        onClick={() => { change.mutate({ action: "check" }); }}>{change.isPending ? "Checking…" : "Check lookup status"}</button>}
      {(result.state === "disconnected" || result.state === "auth_required" ||
        (result.state === "cancelled" && result.reason === "reconnect_required" && !result.confirmation)) &&
        <button type="button" onClick={openConnection}>Open Apollo connection</button>}
      {(result.state === "busy" || ((result.state === "queued" || result.state === "sending") && !polling)) &&
        <button type="button" disabled={lookup.isFetching} onClick={() => { void lookup.refetch(); }}>Refresh status</button>}
    </div>}
  </ApolloDialog>;
}
