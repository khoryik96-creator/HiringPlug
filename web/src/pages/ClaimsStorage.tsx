// Settings > Claims storage (owners and admins): the company's Google Drive,
// OneDrive or Dropbox account that each claim's receipt is uploaded to when
// it is submitted, the top folder, and how the uploads are going.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { claimsKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { fieldErrors } from "../components/forms";
import { localDay } from "../components/placementTimeline";
import { formatWhen } from "../labels";
import "./claims-storage.css";

type Storage = Schemas["ClaimStorage"];
type Provider = Schemas["ClaimStorageProvider"];

export const claimStorageKey = ["settings", "claims-storage"] as const;
export const PROVIDER_NAMES: Record<Provider, string> = { google_drive: "Google Drive", onedrive: "OneDrive", dropbox: "Dropbox" };

/** Where a receipt would go, e.g. Claims/2026-11/Aisyah Rahman/Grab fare/2026-11-03 Grab MYR 23.00.pdf */
export function examplePath(root: string, today = new Date()): string {
  const day = localDay(today.toISOString()); // the user's date, as the folder is named
  const top = root.split(/[/\\]/).map((s) => s.trim()).filter((s) => s && s !== "." && s !== "..").join("/") || "Claims";
  return `${top}/${day.slice(0, 7)}/Aisyah Rahman/Grab fare/${day} Grab MYR 23.00.pdf`;
}

export function ClaimsStorage() {
  const client = useQueryClient();
  const storage = useQuery({
    queryKey: claimStorageKey,
    queryFn: async () => unwrap(await api.GET("/v1/claims/storage")),
    refetchInterval: (q) => (q.state.data?.uploads.pending ? 3000 : false),
  });
  const saved = (s: Storage) => { client.setQueryData(claimStorageKey, s); void client.invalidateQueries({ queryKey: claimsKey }); };
  const connect = useMutation({
    mutationFn: async (provider: Provider) => unwrap(await api.POST("/v1/claims/storage/authorize", { body: { provider } })),
    onSuccess: (v) => { window.location.assign(v.url); },
  });
  const finish = useMutation({
    mutationFn: async (ticket: string) => unwrap(await api.POST("/v1/claims/storage/complete", { body: { ticket } })),
    onSuccess: saved,
  });
  const disconnect = useMutation({
    mutationFn: async () => { unwrap(await api.DELETE("/v1/claims/storage")); },
    onSuccess: () => client.invalidateQueries({ queryKey: claimStorageKey }),
  });
  const retry = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/claims/storage/retry")),
    onSuccess: () => client.invalidateQueries({ queryKey: claimStorageKey }),
  });

  // Back from the provider: finish here, in this signed-in session, and take
  // the one-time ticket out of the address.
  const started = useRef(false);
  const [callbackFailed] = useState(() => new URLSearchParams(window.location.hash.split("?")[1]).get("storage") === "failed");
  useEffect(() => {
    const params = new URLSearchParams(window.location.hash.split("?")[1]);
    const ticket = params.get("ticket");
    if (started.current || params.get("storage") !== "confirm" || !ticket) return;
    started.current = true;
    window.history.replaceState(null, "", "#/settings/claims-storage");
    finish.mutate(ticket);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const s = storage.data;
  return (
    <main className="claims-storage">
      <h2>Claims storage</h2>
      <p className="muted">When a claim is submitted, its receipt is uploaded to the company&apos;s drive, in a folder for the month, the person and the receipt type.</p>
      {callbackFailed && !finish.isSuccess && <p className="error" role="alert">The sign-in was cancelled or expired. Connect again.</p>}
      <ErrorMessage error={storage.error ?? connect.error ?? finish.error ?? disconnect.error ?? retry.error} />
      {finish.isPending && <p role="status">Connecting…</p>}
      {s && (
        <>
          <Account storage={s} busy={connect.isPending || disconnect.isPending}
            onConnect={(p) => { connect.mutate(p); }}
            onDisconnect={() => {
              if (window.confirm("Stop uploading receipts? Files already uploaded stay in the drive.")) disconnect.mutate();
            }} />
          <Folder key={`${s.rootFolder}-${String(s.version)}`} storage={s} onSaved={saved} />
          {s.provider && <Uploads storage={s} retrying={retry.isPending} retried={retry.data?.queued} onRetry={() => { retry.mutate(); }} />}
          <Setup storage={s} />
        </>
      )}
    </main>
  );
}

function Account({ storage: s, busy, onConnect, onDisconnect }: {
  storage: Storage; busy: boolean; onConnect: (p: Provider) => void; onDisconnect: () => void;
}) {
  const [changing, setChanging] = useState(false);
  const choose = !s.provider || changing || s.status === "reconnect";
  return (
    <section className="panel" aria-label="Account">
      <h3>Account</h3>
      {s.provider && (
        <div className="row spread storage-account">
          <div>
            <strong>{PROVIDER_NAMES[s.provider]}</strong> · {s.account}
            <span className={`storage-badge ${s.status}`}>{s.status === "ok" ? "Uploading" : "Connect again"}</span>
            {s.connectedAt && <span className="sub muted small">connected {formatWhen(s.connectedAt)}</span>}
            {s.error && <p className="error small">{s.error}</p>}
          </div>
          <div className="row">
            {!changing && s.status === "ok" && <button type="button" onClick={() => { setChanging(true); }}>Change account</button>}
            <button type="button" className="link danger" disabled={busy} onClick={onDisconnect}>Disconnect</button>
          </div>
        </div>
      )}
      {choose && (
        <>
          {s.provider && <p className="small">{s.status === "reconnect" ? "Connect the account again, or another one:" : "Connect another account:"}</p>}
          <div className="storage-choices">
            {s.options.map((o) => (
              <div key={o.provider} className={`storage-choice${s.provider === o.provider ? " current" : ""}`}>
                <strong>{PROVIDER_NAMES[o.provider]}</strong>
                <button type="button" className={o.ready ? "primary" : undefined} disabled={!o.ready || busy}
                  onClick={() => { onConnect(o.provider); }}>
                  Connect {PROVIDER_NAMES[o.provider]}
                </button>
                {o.problem && <small className="muted">{o.problem}</small>}
              </div>
            ))}
          </div>
          {changing && <button type="button" className="link" onClick={() => { setChanging(false); }}>Cancel</button>}
        </>
      )}
    </section>
  );
}

function Folder({ storage: s, onSaved }: { storage: Storage; onSaved: (s: Storage) => void }) {
  const [root, setRoot] = useState(s.rootFolder);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/claims/storage", { body: { rootFolder: root, version: s.version } })),
    onSuccess: onSaved,
  });
  const errors = fieldErrors(save.error);
  return (
    <section className="panel" aria-label="Folders">
      <h3>Folders</h3>
      <form className="row storage-folder" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
        <label htmlFor="storage-root">Top folder</label>
        <input id="storage-root" value={root} maxLength={200} required aria-invalid={errors.rootFolder ? true : undefined}
          onChange={(e) => { setRoot(e.target.value); }} />
        <button type="submit" disabled={save.isPending || root.trim() === s.rootFolder}>Save</button>
      </form>
      {errors.rootFolder && <small className="error">{errors.rootFolder}</small>}
      <ErrorMessage error={errors.rootFolder ? null : save.error} />
      <p className="small">A receipt goes to <code className="storage-path">{examplePath(root)}</code></p>
      <p className="small muted">Month and day are the receipt&apos;s; the receipt type is its label in Settings &gt; Dropdowns &amp; categories.
        A changed folder applies to receipts submitted from now on.</p>
    </section>
  );
}

function Uploads({ storage: s, retrying, retried, onRetry }: { storage: Storage; retrying: boolean; retried?: number; onRetry: () => void }) {
  const { done, pending, failed } = s.uploads;
  return (
    <section className="panel" aria-label="Uploads">
      <h3>Uploads</h3>
      <p role="status">
        {done} uploaded{pending > 0 && <> · <span>{pending} uploading</span></>}
        {failed > 0 && <> · <span className="error">{failed} failed</span></>}
      </p>
      {failed > 0 && (
        <>
          <p className="small muted">The reason is shown on each claim (Claims &gt; Approvals). </p>
          <button type="button" disabled={retrying || s.status !== "ok"} onClick={onRetry}>Try again</button>
        </>
      )}
      {retried !== undefined && <p className="small muted">{retried} queued.</p>}
    </section>
  );
}

function Setup({ storage: s }: { storage: Storage }) {
  const [copied, setCopied] = useState(false);
  return (
    <details className="panel storage-setup">
      <summary>Setting up the apps</summary>
      <p className="small">Redirect URL to register in each app: <code>{s.callbackUrl}</code>{" "}
        <button type="button" className="link" onClick={() => { void navigator.clipboard.writeText(s.callbackUrl).then(() => { setCopied(true); }); }}>
          {copied ? "Copied" : "Copy"}
        </button>
      </p>
      <ul className="small">
        <li><strong>Google Drive</strong> and <strong>OneDrive</strong> connect through the Google and Microsoft apps in Settings &gt; Email connection.
          Add the redirect URL above to them; for Google, turn on the Google Drive API. HiringPlug sees only the folders it makes.</li>
        <li><strong>Dropbox</strong> uses the server&apos;s Dropbox app (HP_DROPBOX_APP_KEY and HP_DROPBOX_APP_SECRET), with the
          permissions account_info.read and files.content.write.</li>
      </ul>
    </details>
  );
}
