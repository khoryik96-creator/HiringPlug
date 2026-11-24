// The filters across the top of both tabs (who, what, from, to), and the line
// under them: how many entries are shown, and "Download Excel" of everything
// the filters find.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../../api/client";
import { downloadExport } from "../../api/files";
import { usersKey } from "../../api/keys";
import { ErrorMessage } from "../../components/ErrorMessage";
import { AUTOMATIC, CHANGE_KINDS, type Filters, SALARY_KIND, type Tab } from "./filters";

export function FilterBar({ tab, filters, onChange }: { tab: Tab; filters: Filters; onChange: (f: Filters) => void }) {
  const users = useQuery({ queryKey: usersKey, queryFn: async () => unwrap(await api.GET("/v1/users")).items, staleTime: 60_000 });
  const set = (patch: Partial<Filters>) => { onChange({ ...filters, ...patch }); };
  const kinds = tab === "salary"
    ? Object.entries(SALARY_KIND).map(([key, label]) => ({ key, label }))
    : CHANGE_KINDS;
  const people = users.data ?? [];
  const id = `audit-${tab}`;
  return (
    <div className="audit-filters">
      <label className="field" htmlFor={`${id}-who`}>Who
        <select id={`${id}-who`} value={filters.who} onChange={(e) => { set({ who: e.target.value }); }}>
          <option value="">Everyone</option>
          {people.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
          {filters.who && filters.who !== AUTOMATIC && !people.some((u) => u.id === filters.who) && (
            <option value={filters.who}>Someone no longer here</option>
          )}
          {tab === "changes" && <option value={AUTOMATIC}>Automatic</option>}
        </select>
      </label>
      <label className="field" htmlFor={`${id}-what`}>{tab === "salary" ? "Saw" : "What"}
        <select id={`${id}-what`} value={filters.what} onChange={(e) => { set({ what: e.target.value }); }}>
          <option value="">All kinds</option>
          {kinds.map((k) => <option key={k.key} value={k.key}>{k.label}</option>)}
        </select>
      </label>
      <label className="field" htmlFor={`${id}-from`}>From
        <input id={`${id}-from`} type="date" value={filters.from} max={filters.to || undefined}
          onChange={(e) => { set({ from: e.target.value }); }} />
      </label>
      <label className="field" htmlFor={`${id}-to`}>To
        <input id={`${id}-to`} type="date" value={filters.to} min={filters.from || undefined}
          onChange={(e) => { set({ to: e.target.value }); }} />
      </label>
    </div>
  );
}

/** "50 entries shown, more below" and the Excel download of all the filters find. */
export function ListHead({ shown, more, noun, path, body }: {
  shown: number | null; more: boolean; noun: [string, string]; path: string; body: Record<string, unknown>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const download = () => {
    setBusy(true);
    setError(null);
    downloadExport(path, { format: "xlsx", ...body }).catch(setError).finally(() => { setBusy(false); });
  };
  const count = !shown ? "" : `${shown.toLocaleString("en")} ${shown === 1 ? noun[0] : noun[1]}${more ? " shown, more below" : ""}`;
  return (
    <>
      <div className="audit-head">
        <span className="muted" role="status">{count}</span>
        <button type="button" onClick={download} disabled={busy || shown === 0}>{busy ? "Downloading…" : "Download Excel"}</button>
      </div>
      {error !== null && <ErrorMessage error={error} />}
    </>
  );
}
