import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { LeadFindHR } from "../components/LeadFindHR";
import { LeadSources } from "../components/LeadSources";
import { SaveError } from "../components/SaveError";
import { formatDay, keysOf, LEAD_STATUS } from "../labels";

type Lead = Schemas["JobLead"];
type Filters = { status: Schemas["LeadStatus"] | ""; open: "" | "true" | "false"; q: string };

/** Companies that are hiring: leads from sources and ads saved by hand. */
export function Leads({ permissions }: { permissions: Permission[] }) {
  const canManage = permissions.includes("leads:manage");
  const canFindHR = permissions.includes("companies:read") && permissions.includes("contacts:read");
  const [filters, setFilters] = useState<Filters>({ status: "", open: "true", q: "" });
  const list = useQuery({
    queryKey: ["job-leads", filters],
    queryFn: async () => unwrap(await api.GET("/v1/job-leads", {
      params: { query: { status: filters.status || undefined, open: filters.open === "" ? undefined : filters.open === "true", q: filters.q.trim() || undefined } },
    })).items,
  });
  const [adding, setAdding] = useState(false);
  const [sources, setSources] = useState(false);
  // Find HR for one lead, in a panel beside the list (the list as read now, else as it was when opened).
  const [finding, setFinding] = useState<Lead | null>(null);
  const shown = finding ? (list.data?.find((l) => l.id === finding.id) ?? finding) : null;
  const closeFinding = useCallback(() => { setFinding(null); }, []);
  return (
    <main className="list-page">
      {/* Like the other lists: the filters and the buttons beside the title. */}
      <div className="row spread">
        <h2>Leads</h2>
        <div className="row">
          <input aria-label="Search leads" placeholder="Search" value={filters.q} onChange={(e) => { setFilters({ ...filters, q: e.target.value }); }} />
          <select aria-label="Status" value={filters.status} onChange={(e) => { setFilters({ ...filters, status: e.target.value as Filters["status"] }); }}>
            <option value="">Any status</option>
            {keysOf(LEAD_STATUS).map((s) => <option key={s} value={s}>{LEAD_STATUS[s]}</option>)}
          </select>
          <select aria-label="Openings" value={filters.open} onChange={(e) => { setFilters({ ...filters, open: e.target.value as Filters["open"] }); }}>
            <option value="true">Still listed</option><option value="false">Closed</option><option value="">Listed or closed</option>
          </select>
          <button type="button" aria-expanded={sources} onClick={() => { setSources(!sources); }}>Sources</button>
          {canManage && !adding && <button type="button" onClick={() => { setAdding(true); }}>+ Save a lead</button>}
        </div>
      </div>
      {canManage && adding && <SaveAd onDone={() => { setAdding(false); }} />}
      {sources && <LeadSources canManage={canManage} />}
      <div className={shown ? "leads-layout with-hr" : "leads-layout"}>
        <section className="panel" aria-label="Hiring leads">
          <ErrorMessage error={list.error} />
          {list.data?.length === 0 && <p className="muted">No leads here yet.{canManage && " Save an ad you found, or add a source that brings them in."}</p>}
          {list.data && list.data.length > 0 && (
            <table>
              <thead><tr><th>Company</th><th>Job</th><th>Where</th><th>Seen</th><th>Status</th>{canFindHR && <th><span className="visually-hidden">Find HR</span></th>}</tr></thead>
              <tbody>{list.data.map((l) => (
                <LeadRow key={l.id} lead={l} canManage={canManage} open={shown?.id === l.id}
                  onFindHR={canFindHR ? () => { setFinding(shown?.id === l.id ? null : l); } : undefined} />
              ))}</tbody>
            </table>
          )}
        </section>
        {shown && <LeadFindHR key={shown.id} lead={shown} permissions={permissions} onClose={closeFinding} />}
      </div>
    </main>
  );
}

function LeadRow({ lead: l, canManage, open, onFindHR }: { lead: Lead; canManage: boolean; open: boolean; onFindHR: (() => void) | undefined }) {
  const queryClient = useQueryClient();
  const setStatus = useMutation({
    mutationFn: async (status: Schemas["LeadStatus"]) =>
      unwrap(await api.PATCH("/v1/job-leads/{jobLeadId}", { params: { path: { jobLeadId: l.id } }, body: { version: l.version, status } })),
    onSettled: async () => { await queryClient.invalidateQueries({ queryKey: ["job-leads"] }); },
  });
  return (
    <tr className={open ? "lead-open" : undefined}>
      <td>{l.companyId ? <a href={`#/companies/${l.companyId}`}>{l.companyName}</a> : <><strong>{l.companyName}</strong><div className="muted small">not in HiringPlug yet</div></>}</td>
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
      {onFindHR && <td className="lead-find-hr">
        <button type="button" className="doc-btn" aria-pressed={open} aria-label={`Find HR for ${l.companyName}`} onClick={onFindHR}>Find HR</button>
      </td>}
    </tr>
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
