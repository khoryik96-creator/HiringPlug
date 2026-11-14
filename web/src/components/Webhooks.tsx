import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap, type Schemas } from "../api/client";
import { DELIVERY_STATUS, formatWhen } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { SaveError } from "./SaveError";

type Webhook = Schemas["Webhook"];
type Event = Schemas["WebhookEvent"];
const webhooksKey = ["webhooks"] as const;

/** The company's webhooks: add one (its secret shown once), turn on or off, test, remove. */
export function Webhooks() {
  const queryClient = useQueryClient();
  const list = useQuery({ queryKey: webhooksKey, queryFn: async () => unwrap(await api.GET("/v1/webhooks")) });
  const [form, setForm] = useState<{ url: string; events: Event[] }>({ url: "", events: [] });
  const [secret, setSecret] = useState<string | null>(null);
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/webhooks", { body: form })),
    onSuccess: async (created) => {
      setSecret(created.secret);
      setForm({ url: "", events: [] });
      await queryClient.invalidateQueries({ queryKey: webhooksKey });
    },
  });
  const toggle = (e: Event) => {
    setForm({ ...form, events: form.events.includes(e) ? form.events.filter((x) => x !== e) : [...form.events, e] });
  };
  return (
    <section className="panel" aria-label="Webhooks">
      <h3>Webhooks</h3>
      {secret && (
        <div className="warning" role="status">
          <p>Copy the signing secret now: it is not shown again.</p>
          <code className="secret">{secret}</code>{" "}
          <button type="button" className="link" onClick={() => { setSecret(null); }}>Done</button>
        </div>
      )}
      <ErrorMessage error={list.error} />
      {list.data?.items.map((w) => <WebhookItem key={w.id} webhook={w} />)}
      <form className="form-grid" aria-label="New webhook" onSubmit={(e) => { e.preventDefault(); create.mutate(); }}>
        <label className="field full">Address<input required value={form.url} onChange={(e) => { setForm({ ...form, url: e.target.value }); }} /></label>
        <fieldset className="full">
          <legend>Events</legend>
          {list.data?.events.map((e) => (
            <label key={e} className="row"><input type="checkbox" name="events" required={form.events.length === 0} checked={form.events.includes(e)} onChange={() => { toggle(e); }} /> <code>{e}</code></label>
          ))}
        </fieldset>
        <SaveError error={create.error} shown={[]} onReload={() => undefined} />
        <div className="row full"><button type="submit" disabled={create.isPending}>Add webhook</button></div>
      </form>
    </section>
  );
}

function WebhookItem({ webhook: w }: { webhook: Webhook }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const deliveries = useQuery({
    queryKey: [...webhooksKey, w.id, "deliveries"],
    queryFn: async () => unwrap(await api.GET("/v1/webhooks/{webhookId}/deliveries", { params: { path: { webhookId: w.id } } })).items,
    enabled: open,
  });
  const refresh = async () => { await queryClient.invalidateQueries({ queryKey: webhooksKey }); };
  const setActive = useMutation({
    mutationFn: async (active: boolean) =>
      unwrap(await api.PATCH("/v1/webhooks/{webhookId}", { params: { path: { webhookId: w.id } }, body: { version: w.version, active } })),
    onSettled: refresh,
  });
  const ping = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/webhooks/{webhookId}/deliveries", { params: { path: { webhookId: w.id } } })),
    onSuccess: () => { setOpen(true); },
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: async () => { await api.DELETE("/v1/webhooks/{webhookId}", { params: { path: { webhookId: w.id } } }); },
    onSettled: refresh,
  });
  return (
    <article className="webhook" aria-label={w.url}>
      <div className="row spread">
        <div>
          <code>{w.url}</code> {!w.active && <span className="badge withdrawn">Off</span>}
          <div className="muted">{w.events.join(", ")}</div>
        </div>
        <div className="row">
          <button type="button" className="link" onClick={() => { setActive.mutate(!w.active); }}>{w.active ? "Turn off" : "Turn on"}</button>
          <button type="button" className="link" disabled={!w.active || ping.isPending} onClick={() => { ping.mutate(); }}>Send test</button>
          <button type="button" className="link" onClick={() => { setOpen(!open); }}>{open ? "Hide deliveries" : "Deliveries"}</button>
          <button type="button" className="link" onClick={() => { if (window.confirm("Remove this webhook?")) remove.mutate(); }}>Remove</button>
        </div>
      </div>
      <ErrorMessage error={setActive.error ?? ping.error ?? remove.error ?? deliveries.error} />
      {open && deliveries.data && (
        deliveries.data.length === 0 ? <p className="muted">Nothing sent yet.</p> : (
          <table>
            <thead><tr><th>Event</th><th>When</th><th>Status</th><th>Tries</th><th>Answer</th></tr></thead>
            <tbody>
              {deliveries.data.map((d) => (
                <tr key={d.id}>
                  <td><code>{d.event}</code></td><td>{formatWhen(d.createdAt)}</td>
                  <td>{DELIVERY_STATUS[d.status]}{d.nextAttemptAt && d.status === "pending" && <span className="muted"> · again {formatWhen(d.nextAttemptAt)}</span>}</td>
                  <td>{d.attempts}</td><td>{d.lastError ?? (d.responseStatus ? `HTTP ${String(d.responseStatus)}` : "")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )
      )}
    </article>
  );
}
