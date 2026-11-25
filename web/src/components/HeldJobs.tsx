// Settings > System after a restore: the jobs it held because they act
// outside HiringPlug (emails, calendar updates, webhooks, automations). They
// were waiting in the backup, so they may have happened already: each runs
// only once someone releases it, or never if cancelled.
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { formatWhen } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Status = Schemas["SystemStatus"];

export function RestoreNotice({ restore }: { restore: Status["restore"] }) {
  if (!restore) return null;
  const from = restore.fromAnotherServer ? "a backup from another server" : "a backup of this server";
  if (restore.releasedAt) {
    return <p className="muted">Restored {formatWhen(restore.restoredAt)} from {from} ({restore.backupName}).</p>;
  }
  return (
    <p className="warning" role="status">
      Restored {formatWhen(restore.restoredAt)} from {from} ({restore.backupName}): {restore.held} job{restore.held === 1 ? "" : "s"} that
      act outside HiringPlug wait for someone to release or cancel them, in every company on the server. The server&apos;s alert
      emails wait until then.
    </p>
  );
}

export function HeldJobs({ held, label, statusKey }: { held: Status["held"]; label: (kind: string) => string; statusKey: readonly unknown[] }) {
  const queryClient = useQueryClient();
  const done = (st: Status) => { queryClient.setQueryData(statusKey, st); };
  const one = useMutation({
    mutationFn: async ({ id, action }: { id: string; action: "release" | "cancel" }) => unwrap(action === "release"
      ? await api.POST("/v1/settings/system/jobs/{jobId}/release", { params: { path: { jobId: id } } })
      : await api.POST("/v1/settings/system/jobs/{jobId}/cancel", { params: { path: { jobId: id } } })),
    onSuccess: done,
  });
  const all = useMutation({
    mutationFn: async (action: "release" | "cancel") => unwrap(await api.POST("/v1/settings/system/held", { body: { action } })),
    onSuccess: done,
  });
  if (held.length === 0) return null;
  const busy = one.isPending || all.isPending;
  return (
    <section className="panel held-jobs" aria-label="Held after the restore">
      <h3>Held after the restore</h3>
      <p className="muted">
        These were waiting in the backup and reach people or other systems. They may have happened already, before the backup was put back:
        release only what still needs doing; cancel the rest (it shows as failed, never run).
      </p>
      <ErrorMessage error={one.error ?? all.error} />
      <div className="row">
        <button type="button" className="doc-btn" disabled={busy} onClick={() => { all.mutate("release"); }}>Release all {held.length}</button>
        <button type="button" className="doc-btn danger" disabled={busy}
          onClick={() => { if (window.confirm(`Cancel all ${String(held.length)} held jobs? They will never run.`)) all.mutate("cancel"); }}>Cancel all</button>
      </div>
      <div className="table-scroll">
        <table>
          <thead><tr><th>Work</th><th>Queued</th><th>Due</th><th /></tr></thead>
          <tbody>
            {held.map((j) => (
              <tr key={j.id}>
                <td>{label(j.kind)}</td>
                <td className="muted">{formatWhen(j.createdAt)}</td>
                <td className="muted">{formatWhen(j.runAfter)}</td>
                <td className="held-actions">
                  <button type="button" className="link" disabled={busy} onClick={() => { one.mutate({ id: j.id, action: "release" }); }}>Release</button>
                  <button type="button" className="link danger" disabled={busy} onClick={() => { one.mutate({ id: j.id, action: "cancel" }); }}>Cancel</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
