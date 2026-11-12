// Adding or changing an automation: WHEN · IF · THEN, and how its emails go.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { keysOf, STAGE } from "../labels";
import { ACTION, type ActionRow, automationBody, automationForm, type AutomationForm, blankAction, dated, TRIGGER } from "./automationText";
import { Field } from "./Field";
import { fieldErrors } from "./forms";
import { SaveError } from "./SaveError";

export const automationsKey = ["automations"] as const;

const FIELDS = "{{candidate.firstName}}, {{candidate.fullName}}, {{job.title}}, {{company.name}}, {{client.name}}, {{interview.when}}, {{sender.name}}, {{agency.name}}";

export function AutomationEditor({ automation, defaultMode, onDone }: {
  automation?: Schemas["Automation"]; defaultMode: Schemas["AutomationSendMode"]; onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<AutomationForm>(() => automationForm(automation, defaultMode));
  const companies = useQuery({
    queryKey: ["companies", "for-automations"],
    queryFn: async () => unwrap(await api.GET("/v1/companies", { params: { query: { limit: 100 } } })).items,
    staleTime: 60_000,
  });
  const save = useMutation({
    mutationFn: async () => automation
      ? unwrap(await api.PUT("/v1/automations/{automationId}", { params: { path: { automationId: automation.id } }, body: automationBody(form, automation.version) }))
      : unwrap(await api.POST("/v1/automations", { body: automationBody(form) })),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: automationsKey }); onDone(); },
  });
  const remove = useMutation({
    mutationFn: async () => {
      if (!automation) return;
      unwrap(await api.DELETE("/v1/automations/{automationId}", { params: { path: { automationId: automation.id } } }));
    },
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: automationsKey }); onDone(); },
  });
  const errors = fieldErrors(save.error);
  const set = (patch: Partial<AutomationForm>) => { setForm((f) => ({ ...f, ...patch })); };
  const setAction = (i: number, patch: Partial<ActionRow>) => {
    setForm((f) => ({ ...f, actions: f.actions.map((a, j) => (j === i ? { ...a, ...patch } : a)) }));
  };
  const hasEmail = form.actions.some((a) => a.kind !== "task");
  return (
    <form className="panel automation-form" aria-label={automation ? "Change automation" : "New automation"}
      onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <Field label="Name" error={errors.name}><input value={form.name} maxLength={120} onChange={(e) => { set({ name: e.target.value }); }} /></Field>

      <fieldset className="clause">
        <legend>When</legend>
        <div className="row">
          <select aria-label="When" value={form.trigger} onChange={(e) => { set({ trigger: e.target.value as AutomationForm["trigger"] }); }}>
            {keysOf(TRIGGER).map((t) => <option key={t} value={t}>{TRIGGER[t]}</option>)}
          </select>
          {form.trigger === "stage_changed" && (
            <select aria-label="Stage" value={form.stage} onChange={(e) => { set({ stage: e.target.value as AutomationForm["stage"] }); }}>
              {keysOf(STAGE).map((s) => <option key={s} value={s}>{STAGE[s]}</option>)}
            </select>
          )}
        </div>
        {errors.stage && <small className="error">{errors.stage}</small>}
      </fieldset>

      <fieldset className="clause">
        <legend>If</legend>
        <select aria-label="Client" value={form.companyId} onChange={(e) => { set({ companyId: e.target.value }); }}>
          <option value="">Any client</option>
          {companies.data?.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
      </fieldset>

      <fieldset className="clause">
        <legend>Then</legend>
        <div className="row">
          <input type="number" min={0} max={365} className="days" aria-label="Days" value={form.days} onChange={(e) => { set({ days: e.target.value }); }} />
          <span>days</span>
          <select aria-label="Before or after" value={dated(form.trigger) ? form.direction : "after"} disabled={!dated(form.trigger)}
            onChange={(e) => { set({ direction: e.target.value as AutomationForm["direction"] }); }}>
            <option value="after">after</option>
            <option value="before">before</option>
          </select>
        </div>
        {errors.offsetDays && <small className="error">{errors.offsetDays}</small>}
        {form.actions.map((a, i) => (
          <div className="automation-action" key={i}>
            <div className="row spread">
              <select aria-label={`Action ${String(i + 1)}`} value={a.kind} onChange={(e) => { setAction(i, { kind: e.target.value as ActionRow["kind"] }); }}>
                {keysOf(ACTION).map((k) => <option key={k} value={k}>{ACTION[k]}</option>)}
              </select>
              {form.actions.length > 1 && (
                <button type="button" className="link" aria-label={`Remove action ${String(i + 1)}`}
                  onClick={() => { set({ actions: form.actions.filter((_, j) => j !== i) }); }}>✕</button>
              )}
            </div>
            {a.kind === "task" ? (
              <Field label="Task" error={errors[`actions.${String(i)}.title`]}>
                <input value={a.title} maxLength={200} onChange={(e) => { setAction(i, { title: e.target.value }); }} />
              </Field>
            ) : (
              <>
                <Field label="Subject" error={errors[`actions.${String(i)}.subject`]}>
                  <input value={a.subject} maxLength={300} onChange={(e) => { setAction(i, { subject: e.target.value }); }} />
                </Field>
                <Field label="Message" error={errors[`actions.${String(i)}.body`]}>
                  <textarea rows={6} value={a.body} onChange={(e) => { setAction(i, { body: e.target.value }); }} />
                </Field>
              </>
            )}
          </div>
        ))}
        {form.actions.length < 3 && (
          <button type="button" className="link" onClick={() => { set({ actions: [...form.actions, blankAction("task")] }); }}>+ Add another</button>
        )}
        <small className="muted">Fields: {FIELDS}</small>
      </fieldset>

      {hasEmail && (
        <Field label="Emails">
          <select value={form.sendMode} onChange={(e) => { set({ sendMode: e.target.value as AutomationForm["sendMode"] }); }}>
            <option value="approve">Ask before sending</option>
            <option value="send">Send on their own</option>
          </select>
        </Field>
      )}
      <label className="check"><input type="checkbox" checked={form.enabled} onChange={(e) => { set({ enabled: e.target.checked }); }} /> On</label>
      <SaveError error={save.error ?? remove.error} shown={["name", "stage", "offsetDays", ...form.actions.flatMap((_, i) => ["subject", "body", "title"].map((f) => `actions.${String(i)}.${f}`))]} onReload={onDone} />
      <div className="row">
        <button type="submit" disabled={save.isPending}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
        {automation && <button type="button" className="link danger push-right" disabled={remove.isPending} onClick={() => { remove.mutate(); }}>Remove</button>}
      </div>
    </form>
  );
}
