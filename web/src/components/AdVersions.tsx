import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { jobAdsKey } from "../api/keys";
import { countryName } from "../labels";
import { AdText } from "./AdText";
import { BlindRewrite } from "./BlindRewrite";
import { ErrorMessage } from "./ErrorMessage";
import { fieldErrors } from "./forms";
import { useFieldSettings } from "./useFieldSettings";

type Ad = Schemas["JobAd"];
type Version = Schemas["JobAdVersion"];

const TYPES = { docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdf: "application/pdf" } as const;

/** The file name the API also gives: "Finance Manager (blind).pdf". */
export function exportName(title: string, version: Version, format: keyof typeof TYPES): string {
  const name = title.replace(/[^\p{L}\p{N}]+/gu, " ").trim().slice(0, 80) || "Job ad";
  return `${name}${version === "blind" ? " (blind)" : ""}.${format}`;
}

function Exports({ ad, version, title }: { ad: Ad; version: Version; title: string }) {
  const [error, setError] = useState<unknown>(null);
  const get = (format: keyof typeof TYPES) => {
    setError(null);
    downloadFrom(`/v1/job-ads/${ad.id}/export?version=${version}&format=${format}`, exportName(title, version, format), TYPES[format])
      .catch((e: unknown) => { setError(e); });
  };
  return (
    <span className="row">
      <button type="button" className="link" onClick={() => { get("docx"); }}>Word</button>
      <button type="button" className="link" onClick={() => { get("pdf"); }}>PDF</button>
      <ErrorMessage error={error} />
    </span>
  );
}

/** Which of an ad's versions to show. */
export type Shown = "both" | "original" | "blind";

/** An ad as written and its blind version (client not named), side by side (or one of them). */
export function AdVersions({ ad, canEdit, show = "both" }: { ad: Ad; canEdit: boolean; show?: Shown }) {
  const fs = useFieldSettings();
  const factsOf = (location: string | null, salary: string | null) => [
    [location, ad.country ? countryName(ad.country) : null].filter(Boolean).join(", "),
    ad.workType && fs.label("work_type", ad.workType), ad.workArrangement && fs.label("work_arrangement", ad.workArrangement), salary,
  ].filter(Boolean).join(" · ") + (ad.closesOn ? ` · closes ${ad.closesOn}` : "");
  const live = ad.status === "published";
  return (
    <div className={show === "both" ? "ad-columns" : "ad-columns single"}>
      {show !== "blind" && <section aria-label="As written">
        <div className="row spread">
          <h5>As written {live && !ad.publishBlind && <span className="badge published">On the careers page</span>}</h5>
          <Exports ad={ad} version="original" title={ad.title} />
        </div>
        <p className="muted">{factsOf(ad.location, ad.salaryText)}</p>
        <strong>{ad.title}</strong>
        {ad.description ? <AdText text={ad.description} /> : <p className="muted">No text yet.</p>}
      </section>}
      {show !== "original" && <section aria-label="Blind">
        <div className="row spread">
          <h5>Blind {live && ad.publishBlind && <span className="badge published">On the careers page</span>}</h5>
          <Exports ad={ad} version="blind" title={ad.blindTitle} />
        </div>
        <BlindRewrite adId={ad.id} jobId={ad.jobId} canEdit={canEdit} />
        <BlindText ad={ad} canEdit={canEdit} facts={factsOf(ad.blindLocation, ad.blindSalaryText)} />
      </section>}
    </div>
  );
}

function BlindText({ ad, canEdit, facts }: { ad: Ad; canEdit: boolean; facts: string }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ title: ad.blindTitle, text: ad.blindDescription ?? "" });
  const save = useMutation({
    mutationFn: async (body: { blindTitle?: string; blindDescription?: string | null; remakeBlind?: boolean }) =>
      unwrap(await api.PATCH("/v1/job-ads/{adId}", { params: { path: { adId: ad.id } }, body: { version: ad.version, ...body } })),
    onSuccess: async () => {
      setEditing(false);
      await queryClient.invalidateQueries({ queryKey: jobAdsKey(ad.jobId) });
    },
  });
  const errors = fieldErrors(save.error);
  if (editing) {
    return (
      <form aria-label="Edit blind ad" onSubmit={(e) => {
        e.preventDefault();
        save.mutate({ blindTitle: form.title, blindDescription: form.text.trim() ? form.text : null });
      }}>
        <label className="field">Blind title<input required value={form.title} onChange={(e) => { setForm({ ...form, title: e.target.value }); }} /></label>
        {errors.blindTitle && <small className="error">{errors.blindTitle}</small>}
        <label className="field">Blind text<textarea rows={10} value={form.text} onChange={(e) => { setForm({ ...form, text: e.target.value }); }} /></label>
        {errors.blindDescription && <small className="error">{errors.blindDescription}</small>}
        <div className="row">
          <button type="submit" disabled={save.isPending}>Save blind ad</button>
          <button type="button" className="link" onClick={() => { setEditing(false); }}>Cancel</button>
        </div>
        <ErrorMessage error={Object.keys(errors).length ? null : save.error} />
      </form>
    );
  }
  return (
    <>
      <p className="muted">
        {facts}
        {ad.blindEdited && " · edited"}
      </p>
      {ad.blindOutdated && <p className="warning" role="status">The ad changed after this blind text was written. Check it, or make it again from the ad.</p>}
      <strong>{ad.blindTitle}</strong>
      {ad.blindDescription ? <AdText text={ad.blindDescription} /> : <p className="muted">No text yet.</p>}
      {canEdit && (
        <div className="row">
          <button type="button" className="link" onClick={() => {
            setForm({ title: ad.blindTitle, text: ad.blindDescription ?? "" });
            setEditing(true);
          }}>Edit blind ad</button>
          {ad.blindEdited && (
            <button type="button" className="link" disabled={save.isPending} onClick={() => { save.mutate({ remakeBlind: true }); }}>
              Make again from the ad
            </button>
          )}
        </div>
      )}
      <ErrorMessage error={save.error} />
    </>
  );
}
