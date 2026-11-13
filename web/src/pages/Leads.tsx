import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { LeadSources } from "../components/LeadSources";
import { SaveError } from "../components/SaveError";
import { formatDay, keysOf, LEAD_STATUS } from "../labels";

type Lead = Schemas["JobLead"];
type Filters = { status: Schemas["LeadStatus"] | ""; open: "" | "true" | "false"; q: string };

/** Companies that are hiring: leads from sources and ads saved by hand. */
export function Leads({ permissions }: { permissions: Permission[] }) {
  const canManage = permissions.includes("leads:manage");
  const [filters, setFilters] = useState<Filters>({ status: "", open: "true", q: "" });
  const list = useQuery({
    queryKey: ["job-leads", filters],
    queryFn: async () => unwrap(await api.GET("/v1/job-leads", {
      params: { query: { status: filters.status || undefined, open: filters.open === "" ? undefined : filters.open === "true", q: filters.q.trim() || undefined } },
    })).items,
  });
  return (
    <main>
      <h2>Leads</h2>
      <section className="panel" aria-label="Hiring leads">
        <div className="row">
          <label>Search <input value={filters.q} onChange={(e) => { setFilters({ ...filters, q: e.target.value }); }} /></label>
          <label>Status{" "}
            <select value={filters.status} onChange={(e) => { setFilters({ ...filters, status: e.target.value as Filters["status"] }); }}>
              <option value="">Any</option>
              {keysOf(LEAD_STATUS).map((s) => <option key={s} value={s}>{LEAD_STATUS[s]}</option>)}
            </select>
          </label>
          <label>Openings{" "}
            <select value={filters.open} onChange={(e) => { setFilters({ ...filters, open: e.target.value as Filters["open"] }); }}>
              <option value="true">Still listed</option><option value="false">Closed</option><option value="">All</option>
            </select>
          </label>
        </div>
        <ErrorMessage error={list.error} />
        {list.data?.length === 0 && <p className="muted">No leads here yet.</p>}
        {list.data && list.data.length > 0 && (
          <table>
            <thead><tr><th>Company</th><th>Job</th><th>Where</th><th>Seen</th><th>Status</th></tr></thead>
            <tbody>{list.data.map((l) => <LeadRow key={l.id} lead={l} canManage={canManage} />)}</tbody>
          </table>
        )}
        {canManage && <SaveAd />}
      </section>
      <LeadSources canManage={canManage} />
    </main>
  );
}

function LeadRow({ lead: l, canManage }: { lead: Lead; canManage: boolean }) {
  const queryClient = useQueryClient();
  const setStatus = useMutation({
    mutationFn: async (status: Schemas["LeadStatus"]) =>
      unwrap(await api.PATCH("/v1/job-leads/{jobLeadId}", { params: { path: { jobLeadId: l.id } }, body: { version: l.version, status } })),
    onSettled: async () => { await queryClient.invalidateQueries({ queryKey: ["job-leads"] }); },
  });
  return (
    <tr>
      <td>{l.companyId ? <a href={`#/companies/${l.companyId}`}>{l.companyName}</a> : l.companyName}</td>
      <td>{l.url && /^https?:\/\//i.test(l.url) ? <a href={l.url} target="_blank" rel="noreferrer noopener">{l.title}</a> : l.title}{l.notes && <div className="muted">{l.notes}</div>}</td>
      <td>{l.location}</td>
      <td className="muted">{formatDay((l.postedAt ?? l.firstSeenAt).slice(0, 10))}{l.closedAt && <> · closed</>}</td>
      <td>
        {canManage ? (
          <select aria-label={`Status of ${l.title} at ${l.companyName}`} value={l.status} disabled={setStatus.isPending}
            onChange={(e) => { setStatus.mutate(e.target.value as Schemas["LeadStatus"]); }}>
            {keysOf(LEAD_STATUS).map((s) => <option key={s} value={s}>{LEAD_STATUS[s]}</option>)}
          </select>
        ) : LEAD_STATUS[l.status]}
        <ErrorMessage error={setStatus.error} />
      </td>
    </tr>
  );
}

/** Save a job ad found anywhere (LinkedIn, JobStreet, a newspaper) as a lead. */
function SaveAd() {
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
    },
  });
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => { setForm({ ...form, [k]: e.target.value }); };
  return (
    <form className="form-grid" aria-label="Save an ad" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h4 className="full">Save an ad you found</h4>
      <label className="field">Company<input value={form.companyName} onChange={set("companyName")} /></label>
      <label className="field">Job<input value={form.title} onChange={set("title")} /></label>
      <label className="field">Where<input value={form.location} onChange={set("location")} /></label>
      <label className="field">Ad address<input value={form.url} onChange={set("url")} /></label>
      <label className="field full">Notes<input value={form.notes} onChange={set("notes")} /></label>
      <SaveError error={save.error} shown={[]} onReload={() => undefined} />
      <div className="row full"><button type="submit" disabled={save.isPending || !form.companyName.trim() || !form.title.trim()}>Save lead</button></div>
    </form>
  );
}
