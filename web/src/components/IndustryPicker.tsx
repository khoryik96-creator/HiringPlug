import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import "./industries.css";

export type Category = Schemas["Industry"];
export type IndustryChoices = Schemas["IndustrySelections"];
export const industryKey = ["industries"] as const;
export const useIndustries = () => useQuery({ queryKey: industryKey, queryFn: async () => unwrap(await api.GET("/v1/industries")) });
export const categoryLabel = (item: Category, items: Category[]) => {
  const parent = items.find((i) => i.id === item.parentId);
  return parent ? `${parent.label} / ${item.label}` : item.label;
};
const searchable = (s: string) => s.toLocaleLowerCase().replace(/[‐‑‒–—−]/g, "-").replace(/\s*[-/]\s*/g, " ").replace(/\s+/g, " ").trim();
export const categoryMatches = (item: Category, search: string) => searchable([item.label, ...item.aliases].join(" ")).includes(searchable(search));

/** The approved hierarchy browser. All recognition and save validation run in Go. */
export function IndustryPicker({ value, legacy, onChange, candidate, sourceChanged = false }: {
  value?: IndustryChoices; legacy: string; onChange: (value: IndustryChoices, label: string) => void;
  candidate?: { id: string; version: number; currentCompany?: string | null; experience?: unknown[] }; sourceChanged?: boolean;
}) {
  const query = useIndustries();
  const [parent, setParent] = useState("");
  const [search, setSearch] = useState("");
  const [employer, setEmployer] = useState<number | undefined>();
  const suggestion = useQuery({ queryKey: ["industry-suggestion", candidate?.id, candidate?.version, employer], enabled: !!candidate && (!!candidate.currentCompany || !!candidate.experience?.length) && !sourceChanged,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/industry-suggestion", {
      params: { path: { candidateId: candidate?.id ?? "" }, query: employer === undefined ? {} : { employerIndex: employer } },
    })),
  });
  const data = query.data;
  if (!data) return <div className="full"><ErrorMessage error={query.error} />{query.error ? <button type="button" className="secondary" onClick={() => { void query.refetch(); }}>Retry industry catalogue</button> : <span className="muted">Loading industries…</span>}</div>;
  const choices = value ?? { catalogueVersion: data.version, ids: [] };
  const items = data.items;
  const active = items.filter((i) => !i.retired && !items.find((p) => p.id === i.parentId)?.retired);
  const roots = active.filter((i) => !i.parentId && (categoryMatches(i, search) || active.some((c) => c.parentId === i.id && categoryMatches(c, search))));
  const current = roots.find((i) => i.id === parent) ?? roots.find((i) => items.find((c) => c.id === choices.primaryId)?.parentId === i.id) ?? roots[0];
  const change = (ids: string[], primaryId?: string) => {
    const selected = items.find((i) => i.id === primaryId);
    const root = selected?.parentId ? items.find((i) => i.id === selected.parentId) : selected;
    onChange({ catalogueVersion: data.version, ids, ...(primaryId ? { primaryId } : {}) }, root?.label ?? "");
  };
  const toggle = (id: string) => {
    const ids = choices.ids.includes(id) ? choices.ids.filter((i) => i !== id) : [...choices.ids, id];
    change(ids, choices.primaryId === id ? ids[0] : choices.primaryId ?? ids[0]);
  };
  const s = suggestion.data;
  const ready = s?.status === "ready" && s.categoryId && s.candidateVersion === candidate?.version && s.catalogueVersion === data.version;
  const suggested = items.find((i) => i.id === s?.categoryId);
  return <section className="industry-picker full" aria-label="Industries and subindustries">
    <h3>Industries &amp; subindustries</h3>
    <ErrorMessage error={query.error} />
    {candidate && !sourceChanged && <div className="industry-suggestion">
      {s && s.employers.length > 1 && <label className="field">Employer<select value={employer ?? s.employerIndex ?? ""} onChange={(e) => { setEmployer(e.target.value === "" ? undefined : Number(e.target.value)); }}>
        <option value="">Choose employer</option>{s.employers.map((j) => <option key={j.index} value={j.index}>{j.name} · {j.title}{j.current ? " · Current" : ""}</option>)}
      </select></label>}
      {ready && suggested ? <><div><small>Suggested from employer</small><strong>{categoryLabel(suggested, items)}</strong><span className="muted">{s.reason}</span></div>
        <button type="button" className="secondary" onClick={() => { change([...new Set([...choices.ids, suggested.id])], suggested.id); }}>Add as current</button></>
        : <span className="muted">{suggestion.isFetching ? "Checking employer…" : s?.reason ?? "Choose industries manually."}</span>}
      <ErrorMessage error={suggestion.error} />
    </div>}
    {sourceChanged && <p className="muted">Save the employer changes to refresh its suggestion. Manual selection is available.</p>}
    {choices.ids.length === 0 && legacy && <p className="muted">Existing industry: {legacy}</p>}
    <div className="industry-selections">{choices.ids.map((id) => {
      const item = items.find((i) => i.id === id);
      return <div key={id} className="industry-selected"><span>{item ? categoryLabel(item, items) : "Removed category"}{item?.retired && <small> · Retired</small>}</span>
        {choices.primaryId === id ? <span className="chip">Current</span> : <button type="button" className="link" onClick={() => { change(choices.ids, id); }}>Make current</button>}
        <button type="button" className="link" aria-label={`Remove ${item?.label ?? "category"}`} onClick={() => { toggle(id); }}>×</button></div>;
    })}</div>
    {choices.catalogueVersion !== data.version && <p role="status" className="muted">The industry catalogue changed. Review your selections. <button type="button" className="link" onClick={() => { change(choices.ids, choices.primaryId); }}>Use updated catalogue</button></p>}
    <label className="field"><span>Search industries</span><input type="search" aria-label="Search industries" value={search} onChange={(e) => { setSearch(e.target.value); }} /></label>
    <div className="industry-browser"><div className="industry-parents" role="group" aria-label="Industries">{roots.map((i) => <button key={i.id} type="button" className={i.id === current?.id ? "selected" : ""} onClick={() => { setParent(i.id); }}>{i.label}<small>{active.filter((c) => c.parentId === i.id).length}</small></button>)}</div>
      <div className="industry-children">{current ? <><strong>{current.label}</strong>{[current, ...active.filter((i) => i.parentId === current.id && (categoryMatches(current, search) || categoryMatches(i, search)))].map((i) => <label className="row" key={i.id}>
        <input type="checkbox" checked={choices.ids.includes(i.id)} onChange={() => { toggle(i.id); }} /><span>{i.id === current.id ? "Industry only" : i.label}</span></label>)}</> : <p className="muted">No matching industries.</p>}</div>
    </div>
  </section>;
}

export function IndustrySummary({ value, legacy }: { value?: IndustryChoices; legacy?: string | null }) {
  const query = useIndustries();
  if (!value?.ids.length || !query.data) return <>{legacy ?? "—"}</>;
  return <div className="industry-selections">{value.ids.map((id) => {
    const i = query.data.items.find((i) => i.id === id);
    return <span key={id}>{i ? categoryLabel(i, query.data.items) : "Removed category"}{id === value.primaryId && <small> · Current</small>}{i?.retired && <small> · Retired</small>}</span>;
  })}</div>;
}
