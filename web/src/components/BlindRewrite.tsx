// Rewriting a blind ad with the AI, CV Studio's way: the client's identity
// taken out, company size described broadly, the wording paraphrased, the
// tech stack and requirements kept. When it is done it becomes the blind ad.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { jobAdsKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

type Tone = Schemas["BlindRewriteTone"];
type Strength = Schemas["BlindRewriteStrength"];

export const blindRewritesKey = (adId: string) => ["blind-rewrites", adId] as const;

const TONES: { value: Tone; label: string }[] = [
  { value: "professional", label: "Professional" },
  { value: "conversational", label: "Conversational" },
  { value: "concise", label: "Concise" },
];
const STRENGTHS: { value: Strength; label: string }[] = [
  { value: "low", label: "Light" },
  { value: "medium", label: "Balanced" },
  { value: "high", label: "Strong" },
];

/** The latest rewrite (being made, done with what to check, or why not) and, for editors, the form. */
export function BlindRewrite({ adId, jobId, canEdit }: { adId: string; jobId: string; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ tone: "professional" as Tone, strength: "medium" as Strength, removeAlso: "", whyJoinUs: "" });
  const runs = useQuery({
    queryKey: blindRewritesKey(adId),
    queryFn: async () => unwrap(await api.GET("/v1/job-ads/{adId}/blind-rewrites", { params: { path: { adId } } })).items,
    refetchInterval: (q) => (q.state.data?.some((r) => r.status === "queued") ? 3000 : false),
  });
  // A rewrite just finished: the ad's blind version changed.
  const queued = useRef(new Set<string>());
  useEffect(() => {
    const now = new Set(runs.data?.filter((r) => r.status === "queued").map((r) => r.id));
    if ([...queued.current].some((id) => !now.has(id))) void queryClient.invalidateQueries({ queryKey: jobAdsKey(jobId) });
    queued.current = now;
  }, [runs.data, jobId, queryClient]);
  const start = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/job-ads/{adId}/blind-rewrites", {
      params: { path: { adId } },
      body: { tone: form.tone, strength: form.strength, removeAlso: form.removeAlso.trim() || null, whyJoinUs: form.whyJoinUs.trim() || null },
    })),
    onSuccess: async () => {
      setOpen(false);
      await queryClient.invalidateQueries({ queryKey: blindRewritesKey(adId) });
    },
  });
  const latest = runs.data?.[0];
  const busy = latest?.status === "queued";
  return (
    <div className="blind-rewrite">
      {latest?.status === "queued" && <p role="status">Rewriting the blind ad with the AI… this takes a minute.</p>}
      {latest?.status === "done" && latest.warnings.length > 0 && (
        <div className="warning" role="status">
          Rewritten by the AI. Check before sharing:
          <ul>{latest.warnings.map((w) => <li key={w}>{w}</li>)}</ul>
        </div>
      )}
      {latest?.status === "done" && latest.warnings.length === 0 && <p className="muted small">Rewritten by the AI. Read it before sharing.</p>}
      {latest?.status === "failed" && <p className="warning" role="alert">{latest.error}</p>}
      {canEdit && !open && (
        <button type="button" className="link" disabled={busy} onClick={() => { setOpen(true); }}>Rewrite with AI</button>
      )}
      {canEdit && open && (
        <form aria-label="Rewrite blind ad" onSubmit={(e) => { e.preventDefault(); start.mutate(); }}>
          <p className="muted small">
            The AI rewrites the ad for candidates without naming the client: names, brands, addresses, links and contacts go,
            company size and reach are described broadly, the wording is paraphrased; the tech stack and requirements stay.
            It replaces the blind ad (and your own edits to it).
          </p>
          <fieldset className="row">
            <legend>Tone</legend>
            {TONES.map((t) => (
              <label key={t.value}>
                <input type="radio" name="tone" checked={form.tone === t.value} onChange={() => { setForm({ ...form, tone: t.value }); }} /> {t.label}
              </label>
            ))}
          </fieldset>
          <fieldset className="row">
            <legend>Rewording</legend>
            {STRENGTHS.map((s) => (
              <label key={s.value}>
                <input type="radio" name="strength" checked={form.strength === s.value} onChange={() => { setForm({ ...form, strength: s.value }); }} /> {s.label}
              </label>
            ))}
          </fieldset>
          <label className="field">Also take out (optional)
            <input value={form.removeAlso} maxLength={1000} placeholder="e.g. product or project names"
              onChange={(e) => { setForm({ ...form, removeAlso: e.target.value }); }} />
          </label>
          <label className="field">Why join us (optional)
            <textarea rows={2} value={form.whyJoinUs} maxLength={2000} onChange={(e) => { setForm({ ...form, whyJoinUs: e.target.value }); }} />
          </label>
          <div className="row">
            <button type="submit" disabled={start.isPending}>Rewrite</button>
            <button type="button" className="link" onClick={() => { setOpen(false); }}>Cancel</button>
          </div>
        </form>
      )}
      <ErrorMessage error={start.error ?? runs.error} />
    </div>
  );
}
