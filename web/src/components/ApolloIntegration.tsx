// Settings › Integrations: Apollo, for work emails in Find HR. Each recruiter
// connects their own Apollo account here (or from Find HR); the server must
// first be set up with the Apollo sign-in broker.
import { useState } from "react";

import { ApolloConnectionDialog, useHRApollo } from "./FindHRApollo";
import { useCan } from "./useCan";

const STATE: Record<string, string> = {
  connected: "Connected",
  disconnected: "Not connected",
  pending: "Signing in…",
  expired: "Sign-in expired",
  failed: "Sign-in not finished",
  disconnected_locally: "Disconnecting…",
  disabled: "Not set up on this server",
};

export function ApolloIntegration() {
  const canUse = useCan("contacts.apollo:use");
  const connection = useHRApollo(canUse);
  const [open, setOpen] = useState(false);
  if (!canUse) return null;
  const state = connection.data?.state;
  return (
    <section className="panel apollo-integration" aria-labelledby="apollo-heading">
      <div className="row spread">
        <h3 id="apollo-heading">Apollo</h3>
        {state && <span className={`badge apollo-${state}`}>{STATE[state] ?? state}</span>}
      </div>
      <p className="muted small">
        Finds a person&apos;s work email from Companies › Find HR, with your own Apollo account (each lookup may use an Apollo credit).
        {state === "connected" && connection.data?.accountName ? <> Connected as <strong>{connection.data.accountName}</strong>.</> : null}
      </p>
      {!state ? (connection.error ? <p className="error small">Apollo could not be checked just now.</p> : <p className="muted small" role="status">Checking Apollo…</p>)
        : state === "disabled" ? (
        <div className="small">
          <p>To turn it on, whoever runs your HyPlug server sets these two settings (on the API and the worker) and restarts it:</p>
          <pre className="code-block">HP_APOLLO_BROKER_URL=https://&lt;your-worker&gt;.workers.dev{"\n"}HP_APOLLO_KEYS_SECRET=&lt;a random secret of 32+ characters&gt;</pre>
          <p className="muted">The broker is the small Apollo sign-in service in tools/apollo-broker (see its README). Then each recruiter connects here.</p>
        </div>
      ) : (
        <button type="button" className={state === "connected" ? undefined : "primary"} onClick={() => { setOpen(true); }}>
          {state === "connected" ? "Manage connection" : "Connect Apollo"}
        </button>
      )}
      {open && <ApolloConnectionDialog connection={connection} onClose={() => { setOpen(false); }} />}
    </section>
  );
}
