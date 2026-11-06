import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { accessKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { CANDIDATE_SCOPE, keysOf, MODULE, ROLE } from "../labels";

type RoleAccess = Schemas["RoleAccess"];
type Info = Schemas["PermissionInfo"];

/** A role's candidate scope in words, e.g. "Their own candidates, and those on their jobs". */
export function scopeText(a: Pick<RoleAccess, "candidateScope" | "jobCandidates">): string {
  const base = CANDIDATE_SCOPE[a.candidateScope];
  if (a.candidateScope === "all" || !a.jobCandidates) return base;
  return `${base}, and those on ${a.candidateScope === "team" ? "their team's" : "their"} jobs`;
}

/**
 * Roles and access (owners): for each role, what it may do and which
 * candidates it sees. The database enforces both. Changes apply on each
 * person's next click.
 */
export function Access() {
  const settings = useQuery({ queryKey: accessKey, queryFn: async () => unwrap(await api.GET("/v1/access")) });
  const s = settings.data;
  return (
    <main>
      <h2>Roles and access</h2>
      <ErrorMessage error={settings.error} />
      {s?.roles.map((r) => <RoleCard key={`${r.role}-${String(r.version)}`} access={r} catalog={s.permissions} />)}
    </main>
  );
}

function RoleCard({ access: a, catalog }: { access: RoleAccess; catalog: Info[] }) {
  const queryClient = useQueryClient();
  const [perms, setPerms] = useState(new Set(a.permissions));
  const [scope, setScope] = useState(a.candidateScope);
  const [jobs, setJobs] = useState(a.jobCandidates);
  const done = { onSuccess: () => queryClient.invalidateQueries({ queryKey: accessKey }) };
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/access/roles/{role}", {
      params: { path: { role: a.role } }, body: { version: a.version, permissions: [...perms].sort(), candidateScope: scope, jobCandidates: jobs },
    })),
    ...done,
  });
  const reset = useMutation({
    mutationFn: async () => unwrap(await api.DELETE("/v1/access/roles/{role}", { params: { path: { role: a.role } } })),
    ...done,
  });
  if (!a.editable) {
    return (
      <section className="panel" aria-label={ROLE[a.role]}>
        <h3>{ROLE[a.role]}</h3>
        <p className="muted">Owners always have every permission.</p>
      </section>
    );
  }
  const changed = scope !== a.candidateScope || jobs !== a.jobCandidates || perms.size !== a.permissions.length || a.permissions.some((p) => !perms.has(p));
  const modules = [...new Set(catalog.map((p) => p.module))];
  const toggle = (p: Schemas["Permission"], on: boolean) => {
    const next = new Set(perms);
    if (on) next.add(p);
    else next.delete(p);
    setPerms(next);
  };
  return (
    <section className="panel" aria-label={ROLE[a.role]}>
      <details>
        <summary>
          <strong>{ROLE[a.role]}</strong>: {scopeText(a)} · {a.permissions.length} permissions{" "}
          {a.custom ? <span className="badge draft">Changed by you</span> : <span className="badge">Default</span>}
        </summary>
        <fieldset className="access-scope">
          <legend>Which candidates they see</legend>
          {keysOf(CANDIDATE_SCOPE).map((s) => (
            <label key={s}><input type="radio" name={`scope-${a.role}`} checked={scope === s} onChange={() => { setScope(s); }} /> {CANDIDATE_SCOPE[s]}</label>
          ))}
          <label>
            <input type="checkbox" checked={jobs} disabled={scope === "all"} onChange={(e) => { setJobs(e.target.checked); }} />{" "}
            Also the candidates on jobs they (or their team) own
          </label>
        </fieldset>
        {modules.map((m) => (
          <fieldset key={m} className="access-perms">
            <legend>{MODULE[m] ?? m}</legend>
            {catalog.filter((p) => p.module === m).map((p) => (
              <label key={p.permission} title={p.permission}>
                <input type="checkbox" checked={perms.has(p.permission)} disabled={p.permission === "identity.roles:manage"}
                  onChange={(e) => { toggle(p.permission, e.target.checked); }} />{" "}
                {p.description}
                {p.sensitive && <span className="badge draft"> sensitive</span>}
                {!p.defaultRoles.includes(a.role) && perms.has(p.permission) && <span className="muted small"> (not by default)</span>}
              </label>
            ))}
          </fieldset>
        ))}
        <div className="row">
          <button type="button" disabled={!changed || save.isPending} onClick={() => { save.mutate(); }}>Save {ROLE[a.role].toLowerCase()}</button>
          {a.custom && <button type="button" className="link" disabled={reset.isPending} onClick={() => { reset.mutate(); }}>Back to the defaults</button>}
        </div>
        <ErrorMessage error={save.error ?? reset.error} />
      </details>
    </section>
  );
}
