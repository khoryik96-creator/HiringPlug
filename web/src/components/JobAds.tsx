import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type JobOrder, type Schemas, unwrap } from "../api/client";
import { jobAdsKey } from "../api/keys";
import { WORK_ARRANGEMENT, WORK_TYPE } from "../labels";
import { AdText } from "./AdText";
import { ErrorMessage } from "./ErrorMessage";
import { SaveError } from "./SaveError";

type Ad = Schemas["JobAd"];
const STATUS: Record<Ad["status"], string> = { draft: "Draft", published: "Published", closed: "Closed" };
const orNull = (v: string) => (v.trim() === "" ? null : v);

/** A job's ads: the advertised text, separate from the internal job order. */
export function JobAds({ job, canEdit }: { job: JobOrder; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Ad | "new" | null>(null);
  const ads = useQuery({
    queryKey: jobAdsKey(job.id),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/ads", { params: { path: { jobId: job.id } } })).items,
  });
  const setStatus = useMutation({
    mutationFn: async ({ ad, status }: { ad: Ad; status: Ad["status"] }) =>
      unwrap(await api.PATCH("/v1/job-ads/{adId}", { params: { path: { adId: ad.id } }, body: { version: ad.version, status } })),
    onSettled: () => queryClient.invalidateQueries({ queryKey: jobAdsKey(job.id) }),
  });

  return (
    <section className="panel" aria-label="Job ads">
      <div className="row spread">
        <h3>Job ads</h3>
        {canEdit && !editing && <button type="button" onClick={() => { setEditing("new"); }}>Write an ad</button>}
      </div>
      <ErrorMessage error={ads.error ?? setStatus.error} />
      {editing && <AdForm job={job} ad={editing === "new" ? null : editing} onDone={() => { setEditing(null); }} />}
      {ads.data?.length === 0 && !editing && <p className="muted">No ads yet.</p>}
      {ads.data?.map((ad) => (
        <article key={ad.id} className="ad" aria-label={ad.title}>
          <div className="row spread">
            <h4>{ad.title} <span className={`badge ${ad.status}`}>{STATUS[ad.status]}</span></h4>
            {canEdit && editing === null && (
              <div className="row">
                <button type="button" className="link" onClick={() => { setEditing(ad); }}>Edit</button>
                {ad.status !== "published" && (
                  <button type="button" disabled={setStatus.isPending} onClick={() => { setStatus.mutate({ ad, status: "published" }); }}>Publish</button>
                )}
                {ad.status === "published" && (
                  <button type="button" className="link" onClick={() => { setStatus.mutate({ ad, status: "draft" }); }}>Unpublish</button>
                )}
                {ad.status !== "closed" && (
                  <button type="button" className="link" onClick={() => { setStatus.mutate({ ad, status: "closed" }); }}>Close</button>
                )}
              </div>
            )}
          </div>
          <p className="muted">
            {[ad.location, ad.workType && WORK_TYPE[ad.workType], ad.workArrangement && WORK_ARRANGEMENT[ad.workArrangement], ad.salaryText]
              .filter(Boolean).join(" · ")}
            {ad.closesOn && ` · closes ${ad.closesOn}`}
          </p>
          {ad.description ? <AdText text={ad.description} /> : <p className="muted">No text yet.</p>}
        </article>
      ))}
    </section>
  );
}

function AdForm({ job, ad, onDone }: { job: JobOrder; ad: Ad | null; onDone: () => void }) {
  const queryClient = useQueryClient();
  // A new ad starts from the job; an edit saves with the version it was opened with.
  const [opened] = useState(() => ({
    version: ad?.version ?? 0,
    form: {
      title: ad?.title ?? job.title,
      location: ad ? ad.location ?? "" : job.location ?? "",
      workType: ad ? ad.workType ?? "" : job.workType ?? "",
      workArrangement: ad ? ad.workArrangement ?? "" : job.workArrangement ?? "",
      salaryText: ad?.salaryText ?? "",
      closesOn: ad?.closesOn ?? "",
      description: ad ? ad.description ?? "" : job.description ?? "",
    },
  }));
  const [form, setForm] = useState(opened.form);
  const [preview, setPreview] = useState(false);
  const set = (k: keyof typeof form, v: string) => { setForm({ ...form, [k]: v }); };
  const body = {
    title: form.title,
    location: orNull(form.location),
    workType: (orNull(form.workType) as Schemas["WorkType"] | null),
    workArrangement: (orNull(form.workArrangement) as Schemas["WorkArrangement"] | null),
    salaryText: orNull(form.salaryText),
    closesOn: orNull(form.closesOn),
    description: orNull(form.description),
  };
  const save = useMutation({
    mutationFn: async () =>
      ad
        ? unwrap(await api.PATCH("/v1/job-ads/{adId}", { params: { path: { adId: ad.id } }, body: { version: opened.version, ...body } }))
        : unwrap(await api.POST("/v1/jobs/{jobId}/ads", { params: { path: { jobId: job.id } }, body })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: jobAdsKey(job.id) });
      onDone();
    },
  });
  return (
    <form className="panel form-grid" aria-label={ad ? "Edit ad" : "New ad"} onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <label className="field">Ad title<input value={form.title} onChange={(e) => { set("title", e.target.value); }} /></label>
      <label className="field">Location<input value={form.location} onChange={(e) => { set("location", e.target.value); }} /></label>
      <label className="field">Work type
        <select value={form.workType} onChange={(e) => { set("workType", e.target.value); }}>
          <option value="">—</option>
          {Object.entries(WORK_TYPE).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      </label>
      <label className="field">Arrangement
        <select value={form.workArrangement} onChange={(e) => { set("workArrangement", e.target.value); }}>
          <option value="">—</option>
          {Object.entries(WORK_ARRANGEMENT).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      </label>
      <label className="field">Salary shown on the ad<input value={form.salaryText} placeholder="e.g. RM 8,000 - 10,000 a month (leave empty to hide)" onChange={(e) => { set("salaryText", e.target.value); }} /></label>
      <label className="field">Closes on<input type="date" value={form.closesOn} onChange={(e) => { set("closesOn", e.target.value); }} /></label>
      <div className="full">
        <div className="row spread">
          <span>Ad text <span className="muted">(lines starting with "- " become bullet points)</span></span>
          <button type="button" className="link" onClick={() => { setPreview(!preview); }}>{preview ? "Write" : "Preview"}</button>
        </div>
        {preview ? <AdText text={form.description} /> : (
          <textarea aria-label="Ad text" rows={12} value={form.description} onChange={(e) => { set("description", e.target.value); }} />
        )}
      </div>
      <div className="row full">
        <button type="submit" disabled={save.isPending || !form.title.trim()}>{ad ? "Save" : "Save as draft"}</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError error={save.error} shown={[]} onReload={() => { void queryClient.invalidateQueries({ queryKey: jobAdsKey(job.id) }); onDone(); }} />
    </form>
  );
}
