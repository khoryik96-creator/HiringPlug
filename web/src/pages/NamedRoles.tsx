// Roles and access › Named roles (owners): roles of the company's own, each
// built on one of the editable roles ("Junior recruiter" on Recruiter), with
// their own permissions and candidate scope. Given to people on the Team page.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { namedRolesKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { ROLE } from "../labels";
import { AccessFields, scopeText } from "./Access";

type NamedRole = Schemas["NamedRole"];
type Info = Schemas["PermissionInfo"];
const BASES: Schemas["Role"][] = ["admin", "recruiter", "sourcer", "hiring_manager", "viewer"];

export const useNamedRoles = (enabled = true) => useQuery({
  queryKey: namedRolesKey, enabled, queryFn: async () => unwrap(await api.GET("/v1/access/named-roles")),
});

export function NamedRoles({ catalog }: { catalog: Info[] }) {
  const queryClient = useQueryClient();
  const list = useNamedRoles();
  const [adding, setAdding] = useState({ name: "", baseRole: "recruiter" as Schemas["Role"] });
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/access/named-roles", { body: adding })),
    onSuccess: async () => { setAdding({ ...adding, name: "" }); await queryClient.invalidateQueries({ queryKey: namedRolesKey }); },
  });
  return (
    <section className="panel" aria-label="Named roles">
      <h3>Named roles</h3>
      <p className="muted small">
        Roles of your own, each built on a role above (who may manage whom follows it), with their own permissions and candidates.
        Give one to someone on the Team page.
      </p>
      <ErrorMessage error={list.error} />
      {list.data?.items.map((r) => <NamedRoleCard key={`${r.id}-${String(r.version)}`} role={r} catalog={catalog} />)}
      {list.data?.items.length === 0 && <p className="muted">None yet.</p>}
      <form className="row" aria-label="Add a named role" onSubmit={(e) => { e.preventDefault(); if (adding.name.trim()) create.mutate(); }}>
        <input aria-label="Name of the new role" placeholder="e.g. Junior recruiter" maxLength={60} value={adding.name}
          onChange={(e) => { setAdding({ ...adding, name: e.target.value }); }} />
        <label className="row">Built on
          <select aria-label="Built on" value={adding.baseRole} onChange={(e) => { setAdding({ ...adding, baseRole: e.target.value as Schemas["Role"] }); }}>
            {BASES.map((b) => <option key={b} value={b}>{ROLE[b]}</option>)}
          </select>
        </label>
        <button type="submit" disabled={!adding.name.trim() || create.isPending}>Add role</button>
      </form>
      <ErrorMessage error={create.error} />
    </section>
  );
}

function NamedRoleCard({ role: r, catalog }: { role: NamedRole; catalog: Info[] }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(r.name);
  const [perms, setPerms] = useState(new Set(r.permissions));
  const [scope, setScope] = useState(r.candidateScope);
  const [jobs, setJobs] = useState(r.jobCandidates);
  const done = { onSuccess: () => queryClient.invalidateQueries({ queryKey: namedRolesKey }) };
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/access/named-roles/{namedRoleId}", {
      params: { path: { namedRoleId: r.id } },
      body: { name, version: r.version, permissions: [...perms].sort(), candidateScope: scope, jobCandidates: jobs },
    })),
    ...done,
  });
  const remove = useMutation({
    mutationFn: async () => { unwrap(await api.DELETE("/v1/access/named-roles/{namedRoleId}", { params: { path: { namedRoleId: r.id } } })); },
    ...done,
  });
  const changed = name.trim() !== r.name || scope !== r.candidateScope || jobs !== r.jobCandidates
    || perms.size !== r.permissions.length || r.permissions.some((p) => !perms.has(p));
  return (
    <details className="named-role">
      <summary>
        <strong>{r.name}</strong> <span className="muted">built on {ROLE[r.baseRole]}</span>: {scopeText(r)} · {r.permissions.length} permissions ·{" "}
        {r.members === 1 ? "1 person" : `${String(r.members)} people`}
      </summary>
      <label className="field">Name
        <input value={name} maxLength={60} onChange={(e) => { setName(e.target.value); }} />
      </label>
      <AccessFields name={r.id} role={r.baseRole} catalog={catalog} perms={perms} setPerms={setPerms} scope={scope} setScope={setScope} jobs={jobs} setJobs={setJobs} />
      <div className="row">
        <button type="button" disabled={!changed || !name.trim() || save.isPending} onClick={() => { save.mutate(); }}>Save {r.name}</button>
        <button type="button" className="link danger-link" disabled={remove.isPending}
          onClick={() => { if (window.confirm(`Remove the role ${r.name}?`)) remove.mutate(); }}>Remove role</button>
      </div>
      <ErrorMessage error={save.error ?? remove.error} />
    </details>
  );
}
