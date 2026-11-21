import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { LeadSources } from "../components/LeadSources";
import { SaveError } from "../components/SaveError";
import { keysOf, LEAD_STATUS } from "../labels";
import { LeadDiscovery } from "../components/leads/LeadDiscovery";
import { LeadList, LeadReview } from "../components/leads/LeadList";
import { useDebounced } from "../components/useDebounced";

type Filters = { status: Schemas["LeadStatus"] | ""; open: "" | "true" | "false"; q: string };

/** Companies that are hiring: leads from sources and ads saved by hand. */
export function Leads({ permissions }: { permissions: Permission[] }) {
  const canManage = permissions.includes("leads:manage");
  const [filters, setFilters] = useState<Filters>({ status: "", open: "true", q: "" });
  const canDiscover = permissions.includes("candidates:read") && permissions.includes("documents:read");
  const [discover, setDiscover] = useState(false);
  const [favorites, setFavorites] = useState(false);
  const [reviewing, setReviewing] = useState<string>();
  const query = useDebounced(filters.q.trim(), 300);
  const list = useInfiniteQuery({
    queryKey: ["job-leads", "list", filters.status, filters.open, query, favorites],
    enabled: !discover,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => unwrap(await api.GET("/v1/job-leads", {
      signal, params: { query: { status: filters.status || undefined, open: filters.open === "" ? undefined : filters.open === "true",
        q: query || undefined, limit: 50, cursor: pageParam, favorites: favorites || undefined } },
    })),
    getNextPageParam: (page) => page.nextCursor,
  });
  const items = list.data?.pages.flatMap((page) => page.items);
  const [adding, setAdding] = useState(false);
  const [sources, setSources] = useState(false);
  return (
    <main className="list-page">
      {/* Like the other lists: the filters and the buttons beside the title. */}
      <div className="row spread">
        <h2>Leads</h2>
        <div className="row">
          {canDiscover && <button type="button" aria-pressed={discover} onClick={() => { setDiscover(!discover); setReviewing(undefined); }}>
            {discover ? "Back to leads" : "Find jobs from a CV"}
          </button>}
          {!discover && <><input aria-label="Search leads" placeholder="Search" value={filters.q} onChange={(e) => { setFilters({ ...filters, q: e.target.value }); }} />
          <select aria-label="Status" value={filters.status} onChange={(e) => { setFilters({ ...filters, status: e.target.value as Filters["status"] }); }}>
            <option value="">Any status</option>
            {keysOf(LEAD_STATUS).map((s) => <option key={s} value={s}>{LEAD_STATUS[s]}</option>)}
          </select>
          <select aria-label="Openings" value={filters.open} onChange={(e) => { setFilters({ ...filters, open: e.target.value as Filters["open"] }); }}>
            <option value="true">Still listed</option><option value="false">Closed</option><option value="">Listed or closed</option>
          </select>
          <button type="button" aria-pressed={favorites} onClick={() => { setFavorites(!favorites); setReviewing(undefined); }}>Favorites</button>
          <button type="button" aria-expanded={sources} onClick={() => { setSources(!sources); }}>Sources</button>
          {canManage && !adding && <button type="button" onClick={() => { setAdding(true); }}>+ Save a lead</button>}</>}
        </div>
      </div>
      {!discover && canManage && adding && <SaveAd onDone={() => { setAdding(false); }} />}
      {!discover && sources && <LeadSources canManage={canManage} />}
      <div className={reviewing ? "leads-split" : undefined}>
        {discover
          ? <LeadDiscovery canManage={canManage} onReview={setReviewing} />
          : <section className="panel" aria-label="Hiring leads">
            <ErrorMessage error={list.error} />
            {items?.length === 0 && !list.hasNextPage && (
              <p className="muted">{favorites ? "No favorites match these filters." : "No leads match these filters."}</p>
            )}
            {items && items.length > 0 && <LeadList items={items} canManage={canManage} onReview={setReviewing} />}
            {list.hasNextPage && <button type="button" disabled={list.isFetchingNextPage} onClick={() => { void list.fetchNextPage(); }}>Load more leads</button>}
          </section>}
        {reviewing && <LeadReview key={reviewing} id={reviewing} canSeeCompany={permissions.includes("companies:read")}
          onClose={() => { setReviewing(undefined); }} />}
      </div>
    </main>
  );
}

/** Save a job ad found anywhere (LinkedIn, JobStreet, a newspaper) as a lead. */
function SaveAd({ onDone }: { onDone: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ companyName: "", title: "", location: "", url: "", notes: "" });
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/job-leads", {
      body: {
        companyName: form.companyName, title: form.title,
        ...(form.location.trim() ? { location: form.location } : {}), ...(form.url.trim() ? { url: form.url.trim() } : {}),
        ...(form.notes.trim() ? { notes: form.notes } : {}),
      },
    })),
    onSuccess: async () => {
      setForm({ companyName: "", title: "", location: "", url: "", notes: "" });
      await queryClient.invalidateQueries({ queryKey: ["job-leads"] });
      onDone();
    },
  });
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => { setForm({ ...form, [k]: e.target.value }); };
  return (
    <form className="panel form-grid" aria-label="Save an ad" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h4 className="full">Save an ad you found</h4>
      <label className="field">Company<input required value={form.companyName} onChange={set("companyName")} /></label>
      <label className="field">Job<input required value={form.title} onChange={set("title")} /></label>
      <label className="field">Where<input value={form.location} onChange={set("location")} /></label>
      <label className="field">Ad address<input value={form.url} onChange={set("url")} /></label>
      <label className="field full">Notes<input value={form.notes} onChange={set("notes")} /></label>
      <SaveError error={save.error} shown={[]} onReload={() => undefined} />
      <div className="row full">
        <button type="submit" className="primary" disabled={save.isPending}>Save lead</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}
