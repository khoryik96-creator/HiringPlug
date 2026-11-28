import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { CopyButton } from "./CopyButton";
import { ErrorMessage } from "./ErrorMessage";
import { fieldErrors } from "./forms";
import { MicrosoftMark } from "./MicrosoftButton";
import "./sign-in.css";

type Settings = Schemas["MicrosoftSignInSettings"];
const settingsKey = ["agency", "microsoft-sign-in"] as const;

/** Whose Microsoft app signs people in, in words. */
function AppSource({ s }: { s: Settings }) {
  if (s.appSource === "agency") {
    return <p>Uses the Microsoft app already set up for Outlook in <a href="#/email-connection">Settings › Email connection</a>. Add this address to it in Microsoft Entra once (Authentication › Web › Redirect URIs):</p>;
  }
  if (s.appSource === "server") {
    return <p>Uses this server&apos;s Microsoft app, set up by whoever runs HyPlug. Ask them to add this address to it in Microsoft Entra once (Authentication › Web › Redirect URIs):</p>;
  }
  return (
    <p>
      There is no Microsoft app yet. Register one in Microsoft Entra (App registrations › New registration), add it
      in <a href="#/email-connection">Settings › Email connection</a>, and add this address to it as a Web redirect URI:
    </p>
  );
}

/**
 * Roles and access › Microsoft sign-in (owners): let the team sign in with
 * their Microsoft work accounts (only the company's own organisation), and
 * require it of everyone but owners.
 */
export function MicrosoftSignInSettings() {
  const settings = useQuery({ queryKey: settingsKey, queryFn: async () => unwrap(await api.GET("/v1/agency/microsoft-sign-in")) });
  return (
    <section className="panel ms-settings" aria-label="Microsoft sign-in">
      <h3><MicrosoftMark /> Microsoft sign-in</h3>
      <ErrorMessage error={settings.error} />
      {settings.data && <SettingsForm saved={settings.data} />}
    </section>
  );
}

function SettingsForm({ saved }: { saved: Settings }) {
  const cache = useQueryClient();
  const [form, setForm] = useState({ enabled: saved.enabled, organisationId: saved.organisationId ?? "", required: saved.required, connectsOutlook: saved.connectsOutlook });
  const [done, setDone] = useState(false);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/agency/microsoft-sign-in", {
      body: { enabled: form.enabled, organisationId: form.organisationId.trim() || null, required: form.enabled && form.required, connectsOutlook: form.connectsOutlook },
    })),
    onSuccess: (s) => {
      cache.setQueryData(settingsKey, s);
      setForm({ enabled: s.enabled, organisationId: s.organisationId ?? "", required: s.required, connectsOutlook: s.connectsOutlook }); // as the server keeps them
      setDone(true);
    },
  });
  const fields = fieldErrors(save.error);
  const shown = ["enabled", "organisationId", "required"];
  const changed = form.enabled !== saved.enabled || form.required !== saved.required || form.organisationId.trim() !== (saved.organisationId ?? "")
    || form.connectsOutlook !== saved.connectsOutlook;
  const submit = () => {
    const newlyRequired = form.enabled && form.required && !saved.required;
    if (newlyRequired && !window.confirm("Require Microsoft for everyone? Everyone but owners signed in with a password is signed out now, and passwords stop working for them.")) return;
    const organisation = form.organisationId.trim().toLowerCase();
    if (saved.organisationId && organisation !== saved.organisationId && !window.confirm(
      "Change the Microsoft organisation? Every Microsoft account linked to the team is unlinked, and everyone signed in with Microsoft is signed out. "
      + "They link an account of the new organisation at their next sign-in (owners from Your account).")) return;
    if (saved.enabled && !form.enabled && !window.confirm("Turn Microsoft sign-in off? Everyone signed in with Microsoft is signed out now.")) return;
    setDone(false);
    save.mutate();
  };
  const edit = (next: Partial<typeof form>) => { setDone(false); setForm({ ...form, ...next }); };
  return (
    <form className="ms-settings" aria-label="Microsoft sign-in settings" onSubmit={(e) => { e.preventDefault(); submit(); }}>
      <AppSource s={saved} />
      <div className="ms-callback">
        <code>{saved.callbackUrl}</code>
        <CopyButton text={saved.callbackUrl} what="redirect address" />
      </div>
      {/^http:\/\/(?!localhost[:/])/.test(saved.callbackUrl) && (
        <p className="ms-https">Microsoft accepts only https addresses (localhost aside): HyPlug needs its https address (HP_PUBLIC_URL) first.</p>
      )}
      {saved.clientId && <p className="muted small">Application (client) ID: <code>{saved.clientId}</code></p>}
      {saved.acctClaimMissing && (
        <p className="warning ms-claims-warning" role="alert">
          <strong>Add the <code>acct</code> optional claim in Microsoft Entra.</strong> The last sign-in with Microsoft came without it, so
          HyPlug cannot tell your organisation&apos;s guests from its members, and links no account to the team by its address:
          people connect theirs from Your account, or with an invitation or a link from the Team page, until it is added
          (App registrations › your app › Token configuration › Add optional claim › ID › <code>acct</code>).
        </p>
      )}
      <p className="muted small">
        In Microsoft Entra, also add the optional claims <code>acct</code> and <code>xms_edov</code> to the ID token (Token configuration).
        With <code>acct</code>, guests are recognised as such and a first sign-in links an account to the team member with its sign-in name (UPN);
        with <code>xms_edov</code>, also by its email when Microsoft has verified the email&apos;s domain. Without <code>acct</code>, no account is
        linked by its address.
      </p>
      <label className="check">
        <input type="checkbox" checked={form.enabled} onChange={(e) => { edit({ enabled: e.target.checked }); }} />
        <span>Let the team sign in with Microsoft <span className="muted small">(people already on the Team page, with their work account)</span></span>
      </label>
      {fields.enabled && <p className="field-error" role="alert">{fields.enabled}</p>}
      <label className="field">Your Microsoft organisation (only accounts from it can sign in)
        <input value={form.organisationId} placeholder="Directory (tenant) ID, e.g. 72f988bf-86f1-41af-91ab-2d7cd011db47" spellCheck={false}
          autoComplete="off" maxLength={64} aria-invalid={!!fields.organisationId} aria-describedby="ms-org-hint"
          onChange={(e) => { edit({ organisationId: e.target.value }); }} />
        <span id="ms-org-hint" className="muted small">In Microsoft Entra: Overview › Tenant ID.</span>
      </label>
      {fields.organisationId && <p className="field-error" role="alert">Organisation: {fields.organisationId}</p>}
      <label className="check">
        <input type="checkbox" checked={form.enabled && form.required} disabled={!form.enabled} onChange={(e) => { edit({ required: e.target.checked }); }} />
        <span>Everyone must sign in with Microsoft (owners keep their password as a backup)</span>
      </label>
      {fields.required && <p className="field-error" role="alert">{fields.required}</p>}
      <label className="check">
        <input type="checkbox" checked={form.connectsOutlook} onChange={(e) => { edit({ connectsOutlook: e.target.checked }); }} />
        <span>
          Also connect Outlook mail and calendar when people sign in with Microsoft
          <span className="muted small"> (one Microsoft approval instead of three; a mailbox or calendar someone connected to another account is left as it is.
            If your organisation lets only admins approve apps, grant admin consent once in Microsoft Entra: API permissions › Grant admin consent)</span>
        </span>
      </label>
      <ErrorMessage error={Object.keys(fields).some((f) => shown.includes(f)) ? null : save.error} />
      {done && <p role="status" className="muted">Saved. The change is in the audit log, with who made it.</p>}
      <div className="row"><button type="submit" disabled={!changed || save.isPending}>Save</button></div>
    </form>
  );
}
