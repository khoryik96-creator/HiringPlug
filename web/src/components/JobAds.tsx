import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { api, type JobOrder, type Schemas, unwrap } from "../api/client";
import { jobAdsKey } from "../api/keys";
import { careersUrl } from "../careersRoute";
import { regionName } from "../labels";
import { useFieldSettings } from "./useFieldSettings";
import { AdText } from "./AdText";
import { AdCard } from "./JobAdCard";
import { ErrorMessage } from "./ErrorMessage";
import { COUNTRIES } from "./forms";
import { JdDrop, type JdDetails, JD_TYPES } from "./JdDrop";
import { DropZone } from "./DropZone";
import { SaveError } from "./SaveError";

type Ad = Schemas["JobAd"];
const orNull = (v: string) => (v.trim() === "" ? null : v);

/**
 * A job's ads: the advertised text, separate from the internal job order.
 * The panel folds away; each ad is a card that opens to its two versions.
 */
export function JobAds({ job, canEdit }: { job: JobOrder; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Ad | "new" | null>(null);
  const [dropped, setDropped] = useState<File | null>(null); // a job description dropped before writing the ad
  const [shown, setShown] = useState(true);
  const ads = useQuery({
    queryKey: jobAdsKey(job.id),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/ads", { params: { path: { jobId: job.id } } })).items,
  });
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  const setStatus = useMutation({
    mutationFn: async ({ ad, status }: { ad: Ad; status: Ad["status"] }) =>
      unwrap(await api.PATCH("/v1/job-ads/{adId}", { params: { path: { adId: ad.id } }, body: { version: ad.version, status } })),
    onSettled: () => queryClient.invalidateQueries({ queryKey: jobAdsKey(job.id) }),
  });
  const list = ads.data ?? [];
  const live = list.filter((a) => a.status === "published").length;

  return (
    <section className="panel job-ads" aria-label="Job ads">
      <div className="row spread">
        <h3 className="fold-head">
          <button type="button" className="fold-toggle" aria-expanded={shown} onClick={() => { setShown(!shown); }}>Job ads</button>
          {ads.data && (
            <span className="muted small">
              {list.length === 0 ? "none yet" : `${String(list.length)} ad${list.length === 1 ? "" : "s"} · ${String(live)} on the careers page`}
            </span>
          )}
        </h3>
        <div className="row">
          {me.data && (
            <span className="muted small">
              <a href={careersUrl(window.location.origin, me.data.tenant.careersSlug)} target="_blank" rel="noreferrer">Careers page</a>
              {" · "}
              <a href={`${window.location.origin}/v1/public/careers/${me.data.tenant.careersSlug}/jobs.xml`} target="_blank" rel="noreferrer">Job feed</a>
            </span>
          )}
          {canEdit && !editing && <button type="button" onClick={() => { setShown(true); setEditing("new"); }}>Write an ad</button>}
        </div>
      </div>
      {shown && (
        <>
          <ErrorMessage error={ads.error ?? setStatus.error} />
          {canEdit && !editing && (
            <DropZone compact accept={JD_TYPES} label="Drop the client's JD to write an ad"
              hint="PDF, Word or text" onFiles={([f]) => { if (f) { setDropped(f); setEditing("new"); } }} />
          )}
          {editing && <AdForm job={job} ad={editing === "new" ? null : editing} jd={dropped}
            onDone={() => { setEditing(null); setDropped(null); }} />}
          <div className="ad-cards">
            {list.map((ad, i) => (
              <AdCard key={ad.id} ad={ad} startOpen={i === 0 /* the newest; older ones folded */} canEdit={canEdit && editing === null} busy={setStatus.isPending}
                careersSlug={me.data?.tenant.careersSlug}
                onEdit={() => { setEditing(ad); }} onStatus={(status) => { setStatus.mutate({ ad, status }); }} />
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function AdForm({ job, ad, jd, onDone }: { job: JobOrder; ad: Ad | null; jd: File | null; onDone: () => void }) {
  const queryClient = useQueryClient();
  // A new ad starts from the job; an edit saves with the version it was opened with.
  const [opened] = useState(() => ({
    version: ad?.version ?? 0,
    form: {
      title: ad?.title ?? job.title,
      location: ad ? ad.location ?? "" : job.location ?? "",
      country: ad ? ad.country ?? "" : job.country ?? "",
      clientDescription: ad?.clientDescription ?? "",
      publishBlind: ad ? (ad.publishBlind ? "blind" : "original") : "blind",
      workType: ad ? ad.workType ?? "" : job.workType ?? "",
      workArrangement: ad ? ad.workArrangement ?? "" : job.workArrangement ?? "",
      salaryText: ad?.salaryText ?? "",
      closesOn: ad?.closesOn ?? "",
      description: ad ? ad.description ?? "" : job.description ?? "",
    },
  }));
  const [form, setForm] = useState(opened.form);
  const [preview, setPreview] = useState(false);
  const fs = useFieldSettings();
  const set = (k: keyof typeof form, v: string) => { setForm({ ...form, [k]: v }); };
  // What the job description says replaces what is there; what it does not say stays.
  const fillFrom = useCallback((d: JdDetails) => {
    setForm((f) => ({
      ...f,
      title: d.title ?? f.title, location: d.location ?? f.location, country: d.country ?? f.country,
      workType: d.workType ?? f.workType, workArrangement: d.workArrangement ?? f.workArrangement,
      salaryText: d.salaryText ?? f.salaryText, closesOn: d.closesOn ?? f.closesOn, description: d.description || f.description,
    }));
    setPreview(false);
  }, []);
  const body = {
    title: form.title,
    location: orNull(form.location),
    country: orNull(form.country),
    clientDescription: orNull(form.clientDescription),
    publishBlind: form.publishBlind === "blind",
    workType: (orNull(form.workType) as Schemas["WorkType"] | null),
    workArrangement: (orNull(form.workArrangement) as Schemas["WorkArrangement"] | null),
    salaryText: orNull(form.salaryText),
    closesOn: orNull(form.closesOn),
    description: orNull(form.description),
  };
  const save = useMutation({
    mutationFn: async (submitted: typeof body) =>
      ad
        ? unwrap(await api.PATCH("/v1/job-ads/{adId}", { params: { path: { adId: ad.id } }, body: { version: opened.version, ...submitted } }))
        : unwrap(await api.POST("/v1/jobs/{jobId}/ads", { params: { path: { jobId: job.id } }, body: submitted })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: jobAdsKey(job.id) });
      onDone();
    },
  });
  return (
    <form className="panel form-grid" aria-label={ad ? "Edit ad" : "New ad"} onSubmit={(e) => { e.preventDefault(); save.mutate(body); }}>
      <div className="full"><JdDrop jobId={job.id} start={jd} onRead={fillFrom} /></div>
      <label className="field">Ad title<input value={form.title} onChange={(e) => { set("title", e.target.value); }} /></label>
      <label className="field">Location<input value={form.location} onChange={(e) => { set("location", e.target.value); }} /></label>
      <label className="field">Country
        <select value={form.country} onChange={(e) => { set("country", e.target.value); }}>
          <option value="">—</option>
          {[...new Set([...COUNTRIES, ...(form.country ? [form.country] : [])])].map((c) => <option key={c} value={c}>{regionName(c) ?? c}</option>)}
        </select>
      </label>
      <label className="field">Work type
        <select value={form.workType} onChange={(e) => { set("workType", e.target.value); }}>
          <option value="">—</option>
          {fs.options("work_type", form.workType).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
        </select>
      </label>
      <label className="field">Arrangement
        <select value={form.workArrangement} onChange={(e) => { set("workArrangement", e.target.value); }}>
          <option value="">—</option>
          {fs.options("work_arrangement", form.workArrangement).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
        </select>
      </label>
      <label className="field">Salary shown on the ad<input value={form.salaryText} onChange={(e) => { set("salaryText", e.target.value); }} /></label>
      <label className="field">Closes on<input type="date" value={form.closesOn} onChange={(e) => { set("closesOn", e.target.value); }} /></label>
      <label className="field">In the blind ad, the client is
        <input value={form.clientDescription} placeholder="our client" maxLength={200}
          onChange={(e) => { set("clientDescription", e.target.value); }} />
      </label>
      <label className="field">The careers page shows
        <select value={form.publishBlind} onChange={(e) => { set("publishBlind", e.target.value); }}>
          <option value="blind">The blind ad</option>
          <option value="original">The ad as written (names the client)</option>
        </select>
      </label>
      <div className="full">
        <div className="row spread">
          <span>Ad text</span>
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
