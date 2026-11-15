// Settings > Email connection, for owners and admins: the Microsoft and
// Google app details people connect their mailbox through, entered here
// instead of the server's settings file. The secret is saved locked and never
// shown again: only its last four characters.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { fieldErrors } from "./forms";
import { mailboxKey } from "./MailDeliveryNotice";

type Provider = Schemas["MailAppProvider"];
const mailAppsKey = ["settings", "email-apps"] as const;
const NAMES: Record<Provider, string> = { microsoft: "Microsoft", google: "Google" };

export function MailApps() {
  const apps = useQuery({ queryKey: mailAppsKey, queryFn: async () => unwrap(await api.GET("/v1/settings/email-apps")) });
  const [copied, setCopied] = useState(false);
  const a = apps.data;
  return (
    <section className="panel" aria-label="Mail apps">
      <h3>Microsoft and Google apps</h3>
      <ErrorMessage error={apps.error} />
      {a && (
        <>
          {!a.storageReady && <p className="warning">This server cannot store secrets yet: HP_MAIL_KEYS_SECRET must be set on the server first.</p>}
          <p className="small">
            Redirect URI to register: <code>{a.callbackUrl}</code>{" "}
            <button type="button" className="link" onClick={() => { void navigator.clipboard.writeText(a.callbackUrl).then(() => { setCopied(true); }); }}>
              {copied ? "Copied" : "Copy"}
            </button>
          </p>
          <AppForm key={`microsoft-${a.microsoft.updatedAt ?? a.microsoft.source}`} provider="microsoft" app={a.microsoft} ready={a.storageReady} />
          <AppForm key={`google-${a.google.updatedAt ?? a.google.source}`} provider="google" app={a.google} ready={a.storageReady} />
        </>
      )}
    </section>
  );
}

function AppForm({ provider, app, ready }: { provider: Provider; app: Schemas["MailApp"]; ready: boolean }) {
  const client = useQueryClient();
  const [clientId, setClientId] = useState(app.source === "agency" ? app.clientId ?? "" : "");
  const [secret, setSecret] = useState("");
  const done = async (apps: Schemas["MailApps"]) => {
    client.setQueryData(mailAppsKey, apps);
    setSecret("");
    await client.invalidateQueries({ queryKey: mailboxKey });
  };
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/settings/email-apps/{provider}", {
      params: { path: { provider } }, body: { clientId: clientId.trim(), ...(secret.trim() ? { clientSecret: secret.trim() } : {}) },
    })),
    onSuccess: done,
  });
  const remove = useMutation({
    mutationFn: async () => unwrap(await api.DELETE("/v1/settings/email-apps/{provider}", { params: { path: { provider } } })),
    onSuccess: done,
  });
  const errors = fieldErrors(save.error);
  const state = app.source === "agency" ? `Saved · secret ending ${app.secretEnding ?? ""}`
    : app.source === "server" ? "Using the server's settings" : "Not set up";
  return (
    <form className="mail-app" aria-label={`${NAMES[provider]} app`} onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <strong>{NAMES[provider]}</strong>
      <span className={app.source === "none" ? "muted small" : "small"}>{state}</span>
      <input required aria-label={`${NAMES[provider]} client ID`} placeholder="Client ID" value={clientId} disabled={!ready} autoComplete="off" spellCheck={false}
        onChange={(e) => { setClientId(e.target.value); }} />
      <input aria-label={`${NAMES[provider]} client secret`} type="password" autoComplete="new-password" disabled={!ready} value={secret}
        placeholder={app.source === "agency" ? `•••• ${app.secretEnding ?? ""} (leave blank to keep)` : "Client secret"}
        onChange={(e) => { setSecret(e.target.value); }} />
      <button type="submit" disabled={!ready || save.isPending}>Save</button>
      {app.source === "agency" && (
        <button type="button" className="link" disabled={remove.isPending}
          onClick={() => { if (window.confirm(`Remove the ${NAMES[provider]} app? People connected through it must connect again.`)) remove.mutate(); }}>Remove</button>
      )}
      {(errors.clientId ?? errors.clientSecret) && <small className="error full">{errors.clientId ?? errors.clientSecret}</small>}
      <div className="full"><ErrorMessage error={Object.keys(errors).length ? null : save.error ?? remove.error} /></div>
    </form>
  );
}
