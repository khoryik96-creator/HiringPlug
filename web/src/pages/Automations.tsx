// Settings > Automations: emails waiting for approval, the automations (on or
// off), the default for new ones, and what they did.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { AutomationEditor, automationsKey } from "../components/AutomationEditor";
import { ACTION, roughFill, RUN_STATUS, thenText, timingText, toggled, whenText } from "../components/automationText";
import { ErrorMessage } from "../components/ErrorMessage";
import { formatWhen } from "../labels";

const runsKey = ["automation-runs"] as const;

export function Automations({ permissions }: { permissions: Permission[] }) {
  const canManage = permissions.includes("automations:manage");
  const canApprove = permissions.includes("automations:approve");
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<string | null>(null) // an automation's id, or "new";
  const list = useQuery({ queryKey: automationsKey, queryFn: async () => unwrap(await api.GET("/v1/automations")) });
  const runs = useQuery({ queryKey: runsKey, queryFn: async () => unwrap(await api.GET("/v1/automation-runs")).items });
  const toggle = useMutation({
    mutationFn: async ({ a, on }: { a: Schemas["Automation"]; on: boolean }) =>
      unwrap(await api.PUT("/v1/automations/{automationId}", { params: { path: { automationId: a.id } }, body: toggled(a, on) })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: automationsKey }),
  });
  const setDefault = useMutation({
    mutationFn: async (mode: Schemas["AutomationSendMode"]) => unwrap(await api.PUT("/v1/automation-settings", { body: { defaultSendMode: mode } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: automationsKey }),
  });
  const decide = useMutation({
    mutationFn: async ({ id, send }: { id: string; send: boolean }) =>
      unwrap(await api.POST(send ? "/v1/automation-runs/{runId}/send" : "/v1/automation-runs/{runId}/skip", { params: { path: { runId: id } } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: runsKey }),
  });
  const data = list.data;
  const waiting = runs.data?.filter((r) => r.status === "to_approve") ?? [];
  const log = runs.data?.filter((r) => r.status !== "to_approve") ?? [];
  return (
    <main>
      <div className="row spread">
        <h2>Automations</h2>
        {canManage && data && (
          <label className="row tight">Emails of new automations
            <select value={data.settings.defaultSendMode} onChange={(e) => { setDefault.mutate(e.target.value as Schemas["AutomationSendMode"]); }}>
              <option value="approve">Ask before sending</option>
              <option value="send">Send on their own</option>
            </select>
          </label>
        )}
      </div>
      <ErrorMessage error={list.error ?? runs.error ?? toggle.error ?? setDefault.error ?? decide.error} />

      {waiting.length > 0 && (
        <section className="panel" aria-label="To approve">
          <h3>To approve <span className="muted">({waiting.length})</span></h3>
          <ul className="run-list">
            {waiting.map((r) => (
              <li key={r.id}>
                <div>
                  <a href={`#/candidates/${r.candidateId ?? ""}`}><strong>{r.candidateName}</strong></a>{" "}
                  <span className="muted">{r.jobTitle} · {r.companyName}</span>
                  <div className="small">{r.automationName}: {r.actions.filter((x) => x.kind !== "task").map((x) => `${ACTION[x.kind]} “${roughFill(x.subject ?? "", r)}”`).join("; ")}</div>
                </div>
                {canApprove && (
                  <div className="row tight">
                    <button type="button" className="small-button primary" disabled={decide.isPending} onClick={() => { decide.mutate({ id: r.id, send: true }); }}>Send</button>
                    <button type="button" className="small-button" disabled={decide.isPending} onClick={() => { decide.mutate({ id: r.id, send: false }); }}>Skip</button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}

      {editing === "new" && data && <AutomationEditor defaultMode={data.settings.defaultSendMode} onDone={() => { setEditing(null); }} />}
      <section className="panel" aria-label="Your automations">
        <div className="row spread">
          <h3>When · If · Then</h3>
          {canManage && editing !== "new" && <button type="button" onClick={() => { setEditing("new"); }}>+ New automation</button>}
        </div>
        <ul className="automation-list">
          {data?.items.map((a) => editing === a.id ? (
            <li key={a.id}><AutomationEditor automation={a} defaultMode={data.settings.defaultSendMode} onDone={() => { setEditing(null); }} /></li>
          ) : (
            <li key={a.id} className={a.enabled ? "on" : ""}>
              <label className="switch" title={a.enabled ? "On" : "Off"}>
                <input type="checkbox" role="switch" aria-label={`${a.name} on`} checked={a.enabled} disabled={!canManage || toggle.isPending}
                  onChange={(e) => { toggle.mutate({ a, on: e.target.checked }); }} />
                <span aria-hidden="true" />
              </label>
              <div className="automation-sentence">
                <strong>{a.name}</strong>
                <span><b>When</b> {whenText(a).toLowerCase()}{a.companyName && <> · <b>if</b> at {a.companyName}</>} · <b>then</b> {timingText(a)}, {thenText(a.actions)}</span>
              </div>
              {a.actions.some((x) => x.kind !== "task") && <span className="placement-chip">{a.sendMode === "send" ? "Sends" : "Asks first"}</span>}
              {canManage && <button type="button" className="link" onClick={() => { setEditing(a.id); }}>Edit</button>}
            </li>
          ))}
        </ul>
      </section>

      <details className="panel fold" aria-label="Log">
        <summary><span className="fold-title">Log</span> <span className="muted">({log.length})</span></summary>
        {log.length === 0 ? <p className="muted">Nothing yet.</p> : (
          <table className="run-log">
            <thead><tr><th>When</th><th>Automation</th><th>Candidate</th><th>Status</th><th>What happened</th></tr></thead>
            <tbody>
              {log.map((r) => (
                <tr key={r.id}>
                  <td className="num">{formatWhen(r.finishedAt ?? r.dueAt)}</td>
                  <td>{r.automationName}</td>
                  <td>{r.candidateName}<div className="muted small">{r.jobTitle}</div></td>
                  <td><span className={`pill ${r.status === "done" ? "paid" : r.status === "failed" ? "overdue" : "none"}`}>{RUN_STATUS[r.status]}</span></td>
                  <td className="small">{r.detail}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </details>
    </main>
  );
}
