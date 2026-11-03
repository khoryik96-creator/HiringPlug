import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap, type Schemas } from "../api/client";
import { formatWhen, keysOf, LEAD_SOURCE_KIND } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { SaveError } from "./SaveError";

type Source = Schemas["LeadSource"];
export const leadSourcesKey = ["lead-sources"] as const;

const HINT: Record<Schemas["LeadSourceKind"], string> = {
  greenhouse: "Board name or address, e.g. acme or boards.greenhouse.io/acme",
  lever: "Board name or address, e.g. acme or jobs.lever.co/acme",
  ashby: "Board name or address, e.g. acme or jobs.ashbyhq.com/acme",
  smartrecruiters: "Company name on SmartRecruiters, e.g. Acme",
  careers_page: "The careers page address, e.g. https://example.com/careers",
};

/** Where leads come from: add a source, check it now, turn it off, remove it. */
export function LeadSources({ canManage }: { canManage: boolean }) {
  const queryClient = useQueryClient();
  const list = useQuery({
    queryKey: leadSourcesKey,
    queryFn: async () => unwrap(await api.GET("/v1/lead-sources")).items,
    // A check runs in the background: look again while one is due.
    refetchInterval: (q) => (q.state.data?.some((s) => s.active && s.nextCheckAt && Date.parse(s.nextCheckAt) <= Date.now()) ? 3000 : false),
  });
  const [form, setForm] = useState<{ kind: Schemas["LeadSourceKind"]; identifier: string; name: string }>({ kind: "greenhouse", identifier: "", name: "" });
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: leadSourcesKey });
    await queryClient.invalidateQueries({ queryKey: ["job-leads"] });
  };
  const add = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/lead-sources", {
      body: { kind: form.kind, identifier: form.identifier, ...(form.name.trim() ? { name: form.name.trim() } : {}) },
    })),
    onSuccess: async () => { setForm({ ...form, identifier: "", name: "" }); await refresh(); },
  });
  return (
    <section className="panel" aria-label="Sources">
      <h3>Sources</h3>
      <p className="muted">
        Companies&apos; public job boards and careers pages, checked every day. Each opening becomes a lead; one no longer listed is
        marked closed. Sites whose terms forbid it (LinkedIn, JobStreet) are not read: save their ads by hand below.
      </p>
      <ErrorMessage error={list.error} />
      {list.data?.map((s) => <SourceItem key={s.id} source={s} canManage={canManage} onChange={refresh} />)}
      {canManage && (
        <form className="form-grid" aria-label="New source" onSubmit={(e) => { e.preventDefault(); add.mutate(); }}>
          <label className="field">Kind
            <select value={form.kind} onChange={(e) => { setForm({ ...form, kind: e.target.value as Schemas["LeadSourceKind"] }); }}>
              {keysOf(LEAD_SOURCE_KIND).map((k) => <option key={k} value={k}>{LEAD_SOURCE_KIND[k]}</option>)}
            </select>
          </label>
          <label className="field">{form.kind === "careers_page" ? "Address" : "Board"}
            <input value={form.identifier} placeholder={HINT[form.kind]} onChange={(e) => { setForm({ ...form, identifier: e.target.value }); }} />
          </label>
          <label className="field">Company name{form.kind === "careers_page" && " (optional)"}
            <input value={form.name} onChange={(e) => { setForm({ ...form, name: e.target.value }); }} />
          </label>
          <SaveError error={add.error} shown={[]} onReload={() => undefined} />
          <div className="row full"><button type="submit" disabled={add.isPending || !form.identifier.trim()}>Add source</button></div>
        </form>
      )}
    </section>
  );
}

function SourceItem({ source: s, canManage, onChange }: { source: Source; canManage: boolean; onChange: () => Promise<void> }) {
  const check = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/lead-sources/{leadSourceId}/checks", { params: { path: { leadSourceId: s.id } } })),
    onSettled: onChange,
  });
  const toggle = useMutation({
    mutationFn: async () => unwrap(await api.PATCH("/v1/lead-sources/{leadSourceId}", {
      params: { path: { leadSourceId: s.id } }, body: { version: s.version, active: !s.active },
    })),
    onSettled: onChange,
  });
  const remove = useMutation({
    mutationFn: async () => { await api.DELETE("/v1/lead-sources/{leadSourceId}", { params: { path: { leadSourceId: s.id } } }); },
    onSettled: onChange,
  });
  return (
    <article className="lead-source" aria-label={s.name}>
      <div className="row spread">
        <div>
          <strong>{s.name}</strong> <span className="muted">{LEAD_SOURCE_KIND[s.kind]} · {s.identifier}</span>
          {!s.active && <> <span className="badge withdrawn">Off</span></>}
          <div className="muted">
            {s.lastCheckedAt ? <>{s.openLeads} open · checked {formatWhen(s.lastCheckedAt)}</> : "Not checked yet"}
          </div>
          {s.lastError && <p className="warning">Last check: {s.lastError}</p>}
        </div>
        {canManage && (
          <div className="row">
            {s.active && <button type="button" className="link" disabled={check.isPending} onClick={() => { check.mutate(); }}>Check now</button>}
            <button type="button" className="link" onClick={() => { toggle.mutate(); }}>{s.active ? "Turn off" : "Turn on"}</button>
            <button type="button" className="link" onClick={() => { if (window.confirm(`Remove ${s.name}? Its leads stay.`)) remove.mutate(); }}>Remove</button>
          </div>
        )}
      </div>
      <ErrorMessage error={check.error ?? toggle.error ?? remove.error} />
    </article>
  );
}
