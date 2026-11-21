import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { candidatesKey } from "../../api/keys";
import { LEAD_SOURCE_KIND } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { SearchPicker, type Picked } from "../SearchPicker";
import { LeadList } from "./LeadList";

type Input = Schemas["LeadDiscoveryInput"];
type Country = Schemas["LeadDiscoveryCountry"];
type Source = Schemas["LeadDiscoverySource"];

const countries: Record<Country, string> = {
  MY: "Malaysia", SG: "Singapore", AU: "Australia", NZ: "New Zealand", GB: "United Kingdom", US: "United States",
  HK: "Hong Kong", ID: "Indonesia", TH: "Thailand", VN: "Vietnam", PH: "Philippines", IN: "India",
};
const sourceKinds: Source[] = ["greenhouse", "lever", "ashby", "smartrecruiters"];

const select = <T,>(items: T[], item: T, added: boolean): T[] => added ? [...items, item] : items.filter((value) => value !== item);

export function LeadDiscovery({ canManage, onReview }: { canManage: boolean; onReview: (id: string | undefined) => void }) {
  const [candidate, setCandidate] = useState<Picked | null>(null);
  const [document, setDocument] = useState("");
  const [role, setRole] = useState("");
  const [regions, setRegions] = useState<Country[]>(["MY"]);
  const [sources, setSources] = useState<Source[]>(sourceKinds);
  const [criteria, setCriteria] = useState<Input>();

  const cvs = useQuery({
    queryKey: ["documents", candidate?.id],
    enabled: candidate !== null,
    queryFn: async ({ signal }) => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", {
      signal, params: { path: { candidateId: candidate?.id ?? "" } },
    })).items,
  });
  const choices = (cvs.data ?? []).filter((cv) => cv.source !== "formatted" && cv.status === "extracted");
  const selected = document ? choices.find((cv) => cv.id === document) : choices[0];
  const current = criteria?.documentId === selected?.id ? criteria : undefined;
  const matches = useInfiniteQuery({
    queryKey: ["job-leads", "discovery", current],
    enabled: current !== undefined,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ signal, pageParam }) => {
      if (!current) throw new Error("Select a candidate and CV.");
      return unwrap(await api.POST("/v1/job-leads/discover", { signal, body: { ...current, ...(pageParam ? { cursor: pageParam } : {}) } }));
    },
    getNextPageParam: (page) => page.nextCursor,
  });
  const items = [...new Map(matches.data?.pages.flatMap((page) => page.items.map((item) => [item.id, item] as const))).values()];
  const usedRole = matches.data?.pages[0]?.role;
  function clearMatches() {
    setCriteria(undefined);
    onReview(undefined);
  }

  return <section className="panel" aria-label="Find jobs from a CV">
    <form className="form-grid" aria-label="CV job search" onSubmit={(event) => {
      event.preventDefault();
      if (!candidate || !selected || !regions.length || !sources.length) return;
      onReview(undefined);
      setCriteria({ candidateId: candidate.id, documentId: selected.id, countries: regions, sources, ...(role.trim() ? { targetRole: role.trim() } : {}) });
      if (current) void matches.refetch();
    }}>
      <div className="field">
        <label htmlFor="lead-candidate">Candidate</label>
        <SearchPicker id="lead-candidate" value={candidate} queryKey={candidatesKey} placeholder="Search by name"
          noMatch="No matching candidates." onChange={(item) => { setCandidate(item); setDocument(""); clearMatches(); }}
          search={async (q) => unwrap(await api.GET("/v1/candidates", { params: { query: { q, limit: 8 } } })).items.map((c) => ({ id: c.id, name: c.fullName, detail: c.currentTitle }))} />
      </div>
      <div className="field"><label htmlFor="lead-cv">CV</label>
        <select id="lead-cv" value={selected?.id ?? ""} disabled={!choices.length} onChange={(e) => { setDocument(e.target.value); clearMatches(); }}>
          {!choices.length && <option value="">Select a candidate&apos;s CV</option>}
          {choices.map((cv) => <option key={cv.id} value={cv.id}>{cv.fileName}</option>)}
        </select>
      </div>
      {candidate && cvs.isSuccess && !choices.length && <p className="muted small full">
        No original CV has readable text yet. <a href={`#/candidates/${candidate.id}`}>Open candidate</a>
      </p>}
      <label className="field full">Target role (optional)
        <input maxLength={120} value={role} onChange={(event) => { setRole(event.target.value); clearMatches(); }} />
      </label>
      <details className="leads-filter">
        <summary>{regions.length ? regions.map((country) => countries[country]).join(", ") : "Choose countries"}</summary>
        <fieldset className="leads-filter">
          <legend className="visually-hidden">Countries</legend>
        <div className="row">{(Object.keys(countries) as Country[]).map((country) => <label className="row" key={country}>
          <input type="checkbox" checked={regions.includes(country)} onChange={(event) => { setRegions(select(regions, country, event.target.checked)); clearMatches(); }} />
          {countries[country]}
        </label>)}</div>
        </fieldset>
      </details>
      <details className="leads-filter">
        <summary>Job feeds ({sources.length})</summary>
        <fieldset className="leads-filter">
          <legend className="visually-hidden">Job feeds</legend>
        <div className="row">{sourceKinds.map((source) => <label className="row" key={source}>
          <input type="checkbox" checked={sources.includes(source)} onChange={(event) => { setSources(select(sources, source, event.target.checked)); clearMatches(); }} />
          {LEAD_SOURCE_KIND[source]}
        </label>)}</div>
        </fieldset>
      </details>
      <div className="row full">
        <button type="submit" className="primary" disabled={!selected || !regions.length || !sources.length || (current !== undefined && matches.isFetching)}>Find jobs</button>
        {current && matches.isFetching && <span className="muted" role="status">Searching configured feeds…</span>}
      </div>
    </form>
    <ErrorMessage error={cvs.error ?? matches.error} />
    {usedRole && <h3>{usedRole}</h3>}
    {current && matches.isSuccess && items.length === 0 && !matches.hasNextPage && (
      <p className="muted">No matching direct ads in the selected feeds. Check which sources are configured in Leads.</p>
    )}
    {current && items.length > 0 && <LeadList items={items} canManage={canManage} onReview={onReview} />}
    {current && matches.hasNextPage && <button type="button" disabled={matches.isFetchingNextPage}
      onClick={() => { void matches.fetchNextPage(); }}>Load more matches</button>}
  </section>;
}
