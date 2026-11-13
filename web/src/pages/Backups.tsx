// Settings > Backups (owners of the agency that runs the server): the
// nightly backup on or off, "Back up now", and each backup with its size, to
// download to a folder you choose or delete.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { formatWhen } from "../labels";

type Backups = Schemas["Backups"];
type Backup = Schemas["BackupFile"];

export const backupsKey = ["settings", "backups"] as const;

/** "820 KB", "3.4 MB", "1.25 GB". */
export function fileSize(n: number): string {
  if (n < 1024 * 1024) return `${String(Math.max(1, Math.round(n / 1024)))} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
}

export function BackupsPage() {
  const queryClient = useQueryClient();
  const backups = useQuery({
    queryKey: backupsKey,
    queryFn: async () => unwrap(await api.GET("/v1/settings/backups")),
    // While one is asked for or being made, look again often.
    refetchInterval: (q) => (q.state.data?.requestedAt || q.state.data?.last?.status === "running" ? 5_000 : 30_000),
  });
  const update = (b: Backups) => { queryClient.setQueryData(backupsKey, b); };
  const setAuto = useMutation({
    mutationFn: async (auto: boolean) => unwrap(await api.PUT("/v1/settings/backups/auto", { body: { auto } })),
    onSuccess: update,
  });
  const backUpNow = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/settings/backups/run")),
    onSuccess: update,
  });
  const remove = useMutation({
    mutationFn: async (name: string) => unwrap(await api.DELETE("/v1/settings/backups/{backupName}", { params: { path: { backupName: name } } })),
    onSuccess: update,
  });
  const b = backups.data;

  return (
    <main className="backups-page">
      <h2>Backups</h2>
      <ErrorMessage error={backups.error ?? setAuto.error ?? backUpNow.error ?? remove.error} />
      {b && !b.available && (
        <section className="panel">
          <p>Backups of this server are managed by the agency that runs it.</p>
          <p className="muted">A backup holds every agency&apos;s data on the server, so only the owner of the agency that runs it can see, download or delete backups.</p>
        </section>
      )}
      {b?.available && (
        <>
          <section className="panel" aria-label="Nightly backup">
            <h3>Nightly backup</h3>
            <p className="muted">
              A backup is a copy of everything in HiringPlug: candidates, notes and screening calls, salaries, companies, contacts,
              jobs, placements, settings, and every uploaded CV and file. It is kept on the computer HiringPlug runs on (your own
              computer, if HiringPlug runs there); the newest 14 are kept.
            </p>
            <label className="switch-row">
              <input type="checkbox" role="switch" checked={b.auto} disabled={setAuto.isPending}
                onChange={(e) => { setAuto.mutate(e.target.checked); }} />
              <span><strong>Make a backup every night</strong> {b.auto ? "On" : "Off: none are made unless you press Back up now."}</span>
            </label>
            <ServiceLine backups={b} />
            <div className="row">
              <button type="button" disabled={backUpNow.isPending || !!b.requestedAt || b.last?.status === "running"} onClick={() => { backUpNow.mutate(); }}>
                Back up now
              </button>
              {b.requestedAt && <span className="small" role="status">Asked for {formatWhen(b.requestedAt)}: it is made within a minute while the backup service runs.</span>}
              {!b.requestedAt && b.last?.status === "running" && <span className="small" role="status">A backup is being made (started {formatWhen(b.last.startedAt)}).</span>}
            </div>
            {b.last?.status === "failed" && <p className="warning" role="alert">The last backup failed ({formatWhen(b.last.startedAt)}): {b.last.error}</p>}
          </section>
          <BackupList backups={b} onDelete={(name) => { remove.mutate(name); }} deleting={remove.isPending} />
        </>
      )}
    </main>
  );
}

/** Whether the backup service is running: the server's word (its own clock), never this computer's. */
function ServiceLine({ backups: b }: { backups: Backups }) {
  if (b.serviceRunning) return <p className="small">The backup service is running.</p>;
  return (
    <p className="warning small">
      The backup service is not running, so no backup can be made{b.serviceSeenAt ? ` (last seen ${formatWhen(b.serviceSeenAt)})` : ""}.
      With Docker it is the &quot;backup&quot; service; it starts with HiringPlug.
    </p>
  );
}

function BackupList({ backups: b, onDelete, deleting }: { backups: Backups; onDelete: (name: string) => void; deleting: boolean }) {
  return (
    <section className="panel" aria-label="Backups on this server">
      <h3>On this server</h3>
      {b.backups.length === 0 ? <p className="muted">No backups on this server.</p> : (
        <>
          <p className="small" role="status">{b.backups.length} backup{b.backups.length === 1 ? "" : "s"} · {fileSize(b.totalBytes)} in all</p>
          <div className="table-scroll">
          <table>
            <thead><tr><th>Made</th><th>Files</th><th>Size</th><th /></tr></thead>
            <tbody>
              {b.backups.map((backup) => (
                <tr key={backup.name}>
                  <td>{formatWhen(backup.createdAt)}<div className="muted small">{backup.name}</div></td>
                  <td>{backup.files}</td>
                  <td>{fileSize(backup.sizeBytes)}</td>
                  <td className="backup-actions">
                    <DownloadButton backup={backup} />
                    <button type="button" className="link danger" disabled={deleting} onClick={() => {
                      if (window.confirm(`Delete the backup made ${formatWhen(backup.createdAt)}? It cannot be undone.`)) onDelete(backup.name);
                    }}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
          <p className="muted small">
            A download is one .tar file. Keep it somewhere private: it holds every candidate&apos;s details and salaries. To bring one
            back, unpack it into the server&apos;s backups folder and restore it (see the guide).
          </p>
        </>
      )}
    </section>
  );
}

type SaveTarget = { createWritable: () => Promise<WritableStream<Uint8Array>> };
type SavePicker = (options: { suggestedName: string; types?: { description: string; accept: Record<string, string[]> }[] }) => Promise<SaveTarget>;

/** The browser's "Save as" with a folder to choose (Chrome, Edge); undefined elsewhere. */
const savePicker = () => (window as unknown as { showSaveFilePicker?: SavePicker }).showSaveFilePicker;

function DownloadButton({ backup }: { backup: Backup }) {
  const [state, setState] = useState<{ saved?: number; done?: string; error?: unknown }>({});
  const busy = state.saved !== undefined && !state.done && !state.error;

  const download = async () => {
    setState({});
    try {
      const pick = savePicker();
      // Asked first: the browser only opens its window right after the click.
      const target = pick ? await pick({ suggestedName: `${backup.name}.tar`, types: [{ description: "Backup", accept: { "application/x-tar": [".tar"] } }] }) : null;
      const link = unwrap(await api.POST("/v1/settings/backups/{backupName}/link", { params: { path: { backupName: backup.name } } }));
      if (!target) {
        // Other browsers save to their downloads folder (or ask, if set to).
        Object.assign(document.createElement("a"), { href: link.url, download: link.fileName }).click();
        setState({ done: "Your browser is saving it to its downloads folder. To choose the folder, use Chrome or Edge." });
        return;
      }
      setState({ saved: 0 });
      const res = await fetch(link.url);
      if (!res.ok || !res.body) throw new Error(`The download failed (${String(res.status)}). Try again.`);
      let saved = 0;
      let shown = 0; // the progress shown, at most four times a second
      const count = new TransformStream<Uint8Array, Uint8Array>({
        transform(chunk, out) {
          saved += chunk.byteLength;
          if (Date.now() - shown > 250) { shown = Date.now(); setState({ saved }); }
          out.enqueue(chunk);
        },
      });
      await res.body.pipeThrough(count).pipeTo(await target.createWritable());
      setState({ saved, done: "Saved to the folder you chose." });
    } catch (e) {
      if (e instanceof DOMException && e.name === "AbortError") { setState({}); return; } // the person closed the window
      setState({ error: e });
    }
  };

  return (
    <span className="backup-download">
      <button type="button" disabled={busy} onClick={() => { void download(); }}>Download</button>
      {busy && <progress max={backup.sizeBytes} value={state.saved} aria-label={`Downloading ${backup.name}`} />}
      {busy && <span className="small">{fileSize(state.saved ?? 0)} of {fileSize(backup.sizeBytes)}</span>}
      {state.done && <span className="small" role="status">{state.done}</span>}
      <ErrorMessage error={state.error} />
    </span>
  );
}
