// AI candidate search, CV Studio's AI Crawler on our own candidates: the job
// (dropped or pasted, optional), a boolean and filters choose who to
// consider, the AI ranks the best with reasons and gaps, and writes notes.
import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { JdDrop } from "../components/JdDrop";
import { BooleanField, SearchFields } from "../components/search/AdvancedSearch";
import { activeChips, EMPTY, type SearchState, toBody } from "../components/search/candidateSearch";

type Search = Schemas["TalentSearch"];
const RANKS = [10, 15, 20];
const VERDICT: Record<Schemas["TalentVerdict"], string> = { strong: "Strong fit", possible: "Possible fit", weak: "Weak fit" };

export function AiSearch({ canSeeSalary, canReadJd }: { canSeeSalary: boolean; canReadJd: boolean }) {
  const [jd, setJd] = useState("");
  const [draft, setDraft] = useState<SearchState>(EMPTY);
  const [strict, setStrict] = useState(false);
  const [rank, setRank] = useState(15);
  const [shownId, setShownId] = useState<string | null>(null);
  const history = useQuery({ queryKey: ["talent-searches"], queryFn: async () => unwrap(await api.GET("/v1/talent-searches")).items });
  const start = useMutation({
    mutationFn: async () => {
      const search = toBody(draft, "", false); // its page size does not matter here
      return unwrap(await api.POST("/v1/talent-searches", { body: { jd: jd.trim() || undefined, search, strict, rank } }));
    },
    onSuccess: (s) => { setShownId(s.id); void history.refetch(); },
  });
  const shown = useQuery({
    queryKey: ["talent-search", shownId],
    enabled: shownId !== null,
    queryFn: async () => unwrap(await api.GET("/v1/talent-searches/{searchId}", { params: { path: { searchId: shownId ?? "" } } })),
    refetchInterval: (q) => (q.state.data?.status === "queued" ? 3000 : false),
  });
  const ready = jd.trim() !== "" || draft.boolean.trim() !== "";
  const filterCount = activeChips({ ...draft, boolean: "" }, (k) => k).length;

  return (
    <main className="ai-search wide">
      <div className="page-head">
        <div>
          <h2>AI candidate search</h2>
          <p className="muted small">Give the job (or a boolean); the AI shortlists from your candidates with why each fits and what is missing.</p>
        </div>
        <a className="doc-btn" href="#/candidates">← Candidates</a>
      </div>
      <div className="ai-layout">
        <form className="panel ai-form" aria-label="What you need" onSubmit={(e) => { e.preventDefault(); start.mutate(); }}>
          <h3>What you need</h3>
          {canReadJd && <JdDrop readText="Check it, then find candidates." onRead={(d) => { setJd([d.title, d.description].filter(Boolean).join("\n\n")); }} />}
          <label className="adv-field">
            <span className="adv-label">Job description <span className="muted small">{canReadJd ? "or paste it" : "paste it"} · optional with a boolean</span></span>
            <textarea rows={8} value={jd} maxLength={20000} placeholder="Responsibilities, requirements, tech stack…" onChange={(e) => { setJd(e.target.value); }} />
            <span className="muted small">{jd.length.toLocaleString("en-MY")} characters. The AI looks at responsibilities, requirements, tech stack and nice-to-haves, not the company blurb or benefits.</span>
          </label>
          <BooleanField label="Boolean rules / keywords" note="who to consider" value={draft.boolean} onChange={(boolean) => { setDraft((d) => ({ ...d, boolean })); }} />
          <details className="ai-filters">
            <summary>Filters{filterCount > 0 ? ` · ${String(filterCount)} set` : ""} <span className="muted small">status, work authorization, where they live, skills, notice…</span></summary>
            <SearchFields draft={draft} setDraft={setDraft} canSeeSalary={canSeeSalary} />
          </details>
          <div className="ai-options">
            <label className={`toggle-chip${strict ? " on" : ""}`}>
              <input type="checkbox" checked={strict} onChange={(e) => { setStrict(e.target.checked); }} /> Strict must-haves
            </label>
            <label className="row small">The AI ranks the best
              <select value={rank} onChange={(e) => { setRank(Number(e.target.value)); }}>
                {RANKS.map((r) => <option key={r} value={r}>{r}</option>)}
              </select>
            </label>
          </div>
          <div className="adv-actions">
            <button type="submit" disabled={!ready || start.isPending}>{start.isPending ? "Finding…" : "Find candidates"}</button>
            <button type="button" className="link" onClick={() => { setJd(""); setDraft(EMPTY); setStrict(false); }}>Clear</button>
          </div>
          {!ready && <p className="muted small">Give a job description or a boolean to start.</p>}
          <ErrorMessage error={start.error} />
        </form>

        <section className="panel ai-out" aria-label="Shortlist">
          <div className="adv-head">
            <h3>Shortlist</h3>
            {(history.data?.length ?? 0) > 0 && (
              <select aria-label="Earlier searches" value={shownId ?? ""} onChange={(e) => { setShownId(e.target.value || null); }}>
                <option value="">Earlier searches…</option>
                {history.data?.map((h) => <option key={h.id} value={h.id}>{searchLabel(h)}</option>)}
              </select>
            )}
          </div>
          <ErrorMessage error={shown.error} />
          {!shown.data && <p className="muted">Your shortlist shows here: each candidate&apos;s fit, why, and what to ask about.</p>}
          {shown.data && <Shortlist s={shown.data} onUseBoolean={(b) => { setDraft((d) => ({ ...d, boolean: b })); }} />}
        </section>
      </div>
    </main>
  );
}

function searchLabel(s: Search): string {
  const when = new Date(s.createdAt).toLocaleString("en-GB", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
  const what = (s.jd ?? s.boolean ?? "").split("\n")[0]?.slice(0, 40) ?? "";
  return `${when} · ${what}`;
}

function Shortlist({ s, onUseBoolean }: { s: Search; onUseBoolean: (b: string) => void }) {
  if (s.status === "queued") {
    return <p role="status" className="ai-wait">The AI is ranking {s.sent} of {s.considered} candidates found… this takes a minute.</p>;
  }
  if (s.status === "failed") return <p className="warning" role="alert">{s.error ?? "It could not be ranked."}</p>;
  const mark = s.boolean ?? s.suggestedBoolean ?? "";
  return (
    <div className="shortlist">
      <p className="muted small">{s.considered} found by the search · the AI ranked {s.sent}{s.strict ? " · strict must-haves" : ""}</p>
      {s.summary && <p className="ai-notes">{s.summary}</p>}
      {(s.mustHaves.length > 0 || s.niceToHaves.length > 0) && (
        <div className="ai-needs">
          {s.mustHaves.length > 0 && <div><span className="multi-label">Must have</span> {s.mustHaves.map((m) => <span key={m} className="chip">{m}</span>)}</div>}
          {s.niceToHaves.length > 0 && <div><span className="multi-label">Nice to have</span> {s.niceToHaves.map((m) => <span key={m} className="chip soft">{m}</span>)}</div>}
        </div>
      )}
      {s.suggestedBoolean && (
        <div className="ai-boolean">
          <span className="multi-label">Boolean for this job</span> <code>{s.suggestedBoolean}</code>
          <button type="button" className="link small" onClick={() => { onUseBoolean(s.suggestedBoolean ?? ""); }}>Use it</button>
        </div>
      )}
      <ol className="fit-list">
        {s.results.map((r) => (
          <li key={r.candidateId} className={`fit-card ${r.verdict}`}>
            <span className={`fit-score ${r.verdict}`} title={VERDICT[r.verdict]}>{r.fit}</span>
            <div className="fit-main">
              <div className="fit-head">
                <a href={`#/candidates/${r.candidateId}${mark ? `?hl=${encodeURIComponent(mark)}` : ""}`}>{r.name}</a>
                {r.currentTitle && <span className="muted small"> · {r.currentTitle}</span>}
                <span className={`fit-verdict ${r.verdict}`}>{VERDICT[r.verdict]}</span>
              </div>
              <div className="fit-notes">
                {r.reasons.length > 0 && <ul className="fit-good">{r.reasons.map((x) => <li key={x}>{x}</li>)}</ul>}
                {r.gaps.length > 0 && <ul className="fit-gap">{r.gaps.map((x) => <li key={x}>{x}</li>)}</ul>}
              </div>
            </div>
          </li>
        ))}
      </ol>
      {s.results.length === 0 && <p className="muted">Nobody to show.</p>}
    </div>
  );
}
