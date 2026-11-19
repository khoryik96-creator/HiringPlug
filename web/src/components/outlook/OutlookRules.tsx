import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Schemas, unwrap } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import { Field } from "../Field";
import { fieldErrors } from "../forms";

const rulesKey = ["outlook", "rules"] as const;

export function OutlookRules() {
  const client = useQueryClient();
  const list = useQuery({
    queryKey: rulesKey,
    queryFn: async () => unwrap(await api.GET("/v1/outlook/rules")),
    retry: false,
  });
  const connect = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/mailbox/authorize", { body: { provider: "microsoft", inboxRules: true } })),
    onSuccess: ({ url }) => { window.location.assign(url); },
  });
  const change = useMutation({
    mutationFn: async ({ id, enabled }: { id: string; enabled?: boolean }) => {
      const params = { path: { ruleId: id } };
      if (enabled === undefined) unwrap(await api.DELETE("/v1/outlook/rules/{ruleId}", { params }));
      else unwrap(await api.PATCH("/v1/outlook/rules/{ruleId}", { params, body: { enabled } }));
    },
    onSuccess: () => client.invalidateQueries({ queryKey: rulesKey }),
  });
  const needsAccess = list.error instanceof ApiError && list.error.code === "OUTLOOK_RULES_ACCESS";
  return <main className="outlook-page">
    <div className="row spread">
      <h2>Inbox rules</h2>
      <a className="button" href="#/outlook">Back to Outlook Mail</a>
    </div>
    <ErrorMessage error={change.error ?? connect.error} />
    {list.isPending && <p className="muted" role="status">Loading inbox rules…</p>}
    {needsAccess ? <section className="panel">
      <h3>Allow inbox rules</h3>
      <p>Sign in to the same Microsoft mailbox to manage its rules. Your administrator may need to approve this permission.</p>
      <button className="primary" type="button" disabled={connect.isPending} onClick={() => { connect.mutate(); }}>Allow inbox rules</button>
    </section> : <ErrorMessage error={list.error}>
      <button type="button" disabled={list.isFetching} onClick={() => { void list.refetch(); }}>Try again</button>
    </ErrorMessage>}
    {list.data && <>
      <section className="panel" aria-label="Outlook rules in order">
        <h3>Outlook rules</h3>
        {list.data.items.length === 0 ? <p className="muted">No inbox rules yet.</p> : <div className="table-scroll">
          <table>
            <thead><tr><th>Rule</th><th>Move to</th><th>Status</th><th>Actions</th></tr></thead>
            <tbody>{list.data.items.map((rule) => <tr key={rule.id}>
              <td>{rule.name}</td><td>{rule.destination || "—"}</td>
              <td>{rule.enabled ? "On" : "Off"}{rule.hasError && <span className="warning"> · Check in Outlook</span>}</td>
              <td>{rule.readOnly ? <span className="muted">Read only</span> : <div className="row outlook-actions">
                <button type="button" aria-label={`${rule.enabled ? "Turn off" : "Turn on"} ${rule.name}`} disabled={change.isPending}
                  onClick={() => { change.mutate({ id: rule.id, enabled: !rule.enabled }); }}>{rule.enabled ? "Turn off" : "Turn on"}</button>
                <button type="button" aria-label={`Delete ${rule.name}`} disabled={change.isPending}
                  onClick={() => { if (window.confirm(`Delete the Outlook rule “${rule.name}”?`)) change.mutate({ id: rule.id }); }}>Delete</button>
              </div>}</td>
            </tr>)}</tbody>
          </table>
        </div>}
      </section>
      <NewRule folders={list.data.folders} />
    </>}
  </main>;
}

function NewRule({ folders }: { folders: Schemas["OutlookRuleFolder"][] }) {
  const client = useQueryClient();
  const [name, setName] = useState("");
  const [from, setFrom] = useState("");
  const [subject, setSubject] = useState("");
  const [folderId, setFolderId] = useState("");
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/outlook/rules", {
      body: { name: name.trim(), folderId, fromAddress: from.trim() || undefined, subjectContains: subject.trim() || undefined },
    })),
    onSuccess: async () => {
      setName(""); setFrom(""); setSubject(""); setFolderId("");
      await client.invalidateQueries({ queryKey: rulesKey });
    },
  });
  const errors = fieldErrors(create.error);
  return <section className="panel" aria-label="New inbox rule">
    <h3>New rule</h3>
    {folders.length === 0 ? <p className="muted">Sync Outlook Mail to load your folders.</p> : <form className="outlook-rule-form"
      onSubmit={(event) => { event.preventDefault(); if (!create.isPending) create.mutate(); }}>
      <Field label="Rule name" error={errors.name}>
        <input required maxLength={120} value={name} onChange={(e) => { setName(e.target.value); }} />
      </Field>
      <Field label="Move to" error={errors.folderId}>
        <select required value={folderId} onChange={(e) => { setFolderId(e.target.value); }}>
          <option value="">Choose a folder</option>
          {folders.map((folder) => <option key={folder.id} value={folder.id}>{folder.name}</option>)}
        </select>
      </Field>
      <Field label="Sender address" error={errors.fromAddress}>
        <input type="email" maxLength={254} value={from} onChange={(e) => { setFrom(e.target.value); }} />
      </Field>
      <Field label="Subject contains" error={errors.subjectContains}>
        <input maxLength={120} value={subject} onChange={(e) => { setSubject(e.target.value); }} />
      </Field>
      <p className="muted">Enter a sender, subject text, or both. If you enter both, both must match.</p>
      <ErrorMessage error={create.error} />
      <div><button className="primary" type="submit" disabled={create.isPending || !name.trim() || !folderId || !(from.trim() || subject.trim())}>
        {create.isPending ? "Creating…" : "Create rule"}
      </button></div>
    </form>}
  </section>;
}
