import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap, type Schemas } from "../api/client";
import { API_KEY_ROLE, formatWhen, keysOf } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

const apiKeysKey = ["api-keys"] as const;

/** The company's API keys: create one (shown once), revoke one. */
export function ApiKeys() {
  const queryClient = useQueryClient();
  const list = useQuery({ queryKey: apiKeysKey, queryFn: async () => unwrap(await api.GET("/v1/api-keys")).items });
  const [form, setForm] = useState<{ name: string; role: Schemas["ApiKeyRole"] }>({ name: "", role: "recruiter" });
  const [newKey, setNewKey] = useState<string | null>(null);
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/api-keys", { body: form })),
    onSuccess: async (created) => {
      setNewKey(created.key);
      setForm({ name: "", role: "recruiter" });
      await queryClient.invalidateQueries({ queryKey: apiKeysKey });
    },
  });
  const revoke = useMutation({
    mutationFn: async (id: string) => { await api.DELETE("/v1/api-keys/{apiKeyId}", { params: { path: { apiKeyId: id } } }); },
    onSettled: async () => { await queryClient.invalidateQueries({ queryKey: apiKeysKey }); },
  });
  return (
    <section className="panel" aria-label="API keys">
      <h3>API keys</h3>
      <p className="muted small">Header: <code>Authorization: Bearer &lt;key&gt;</code> · docs at <code>/docs</code></p>
      {newKey && (
        <div className="warning" role="status">
          <p>Copy this key now: it is not shown again.</p>
          <code className="secret">{newKey}</code>{" "}
          <button type="button" className="link" onClick={() => { setNewKey(null); }}>Done</button>
        </div>
      )}
      <ErrorMessage error={list.error ?? revoke.error} />
      {list.data?.length === 0 && <p className="muted">No API keys.</p>}
      {list.data && list.data.length > 0 && (
        <table>
          <thead><tr><th>Name</th><th>Acts as</th><th>Key</th><th>Last used</th><th /></tr></thead>
          <tbody>
            {list.data.map((k) => (
              <tr key={k.id}>
                <td>{k.name}{k.createdByName && <span className="muted"> · by {k.createdByName}</span>}</td>
                <td>{API_KEY_ROLE[k.role]}</td><td><code>{k.prefix}…</code></td>
                <td>{k.lastUsedAt ? formatWhen(k.lastUsedAt) : "Never"}</td>
                <td>
                  <button type="button" className="link" disabled={revoke.isPending}
                    onClick={() => { if (window.confirm(`Revoke "${k.name}"? Anything using it stops working.`)) revoke.mutate(k.id); }}>
                    Revoke
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <form className="row" aria-label="New API key" onSubmit={(e) => { e.preventDefault(); create.mutate(); }}>
        <label>Name <input required value={form.name} onChange={(e) => { setForm({ ...form, name: e.target.value }); }} /></label>
        <label>Acts as{" "}
          <select value={form.role} onChange={(e) => { setForm({ ...form, role: e.target.value as Schemas["ApiKeyRole"] }); }}>
            {keysOf(API_KEY_ROLE).map((r) => <option key={r} value={r}>{API_KEY_ROLE[r]}</option>)}
          </select>
        </label>
        <button type="submit" disabled={create.isPending}>Create key</button>
      </form>
      <ErrorMessage error={create.error} />
    </section>
  );
}
