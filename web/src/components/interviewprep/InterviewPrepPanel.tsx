// Interview prep material the AI writes for a job, from its description, the
// client and the questions recorded at its interviews: short cards to scan,
// each question with why it is asked, how to approach it, example phrasings,
// trade-offs and what to avoid. Kept with the job; archived when old.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { downloadFrom } from "../../api/files";
import { formatWhen } from "../../labels";
import { ArchiveIcon, IconButton, RefreshIcon } from "../ActionIcons";
import { copyText } from "../CopyButton";
import { ErrorMessage } from "../ErrorMessage";
import "./interviewprep.css";

type Prep = Schemas["InterviewPrep"];
type Content = Schemas["InterviewPrepContent"];
type Item = Schemas["InterviewPrepItem"];

const prepsKey = (jobId: string) => ["interview-preps", jobId] as const;
const FILE_TYPES = { docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdf: "application/pdf" } as const;

/** The material as plain text, to paste in an email or a note. */
export function prepText(title: string, c: Content): string {
  const list = (head: string, xs: string[], pad = "") => (xs.length ? [`${pad}${head}:`, ...xs.map((x) => `${pad}  - ${x}`)] : []);
  const item = (it: Item, n: number) => [
    `${String(n)}. ${it.question}${it.fromInterviews ? " (asked at interview)" : ""}`,
    ...(it.whyAsked ? [`  Why they ask: ${it.whyAsked}`] : []),
    ...list("How to approach", it.approach, "  "),
    ...list("Say it like", it.phrasing, "  "),
    ...list("Trade-offs", it.tradeOffs, "  "),
    ...list("Avoid", it.pitfalls, "  "),
  ].join("\n");
  return [
    `Interview prep: ${title}`, c.summary, ...list("Focus on", c.focusAreas),
    ...(c.softSkills.length ? ["", "SOFT SKILLS", ...c.softSkills.map((it, i) => item(it, i + 1))] : []),
    ...(c.technical.length ? ["", "TECHNICAL", ...c.technical.map((it, i) => item(it, i + 1))] : []),
    ...(c.questionsToAsk.length ? ["", ...list("Questions to ask them", c.questionsToAsk)] : []),
  ].join("\n");
}

export function InterviewPrepPanel({ jobId, jobTitle, canManage }: { jobId: string; jobTitle: string; canManage: boolean }) {
  const client = useQueryClient();
  const [archived, setArchived] = useState(false);
  const [focus, setFocus] = useState("");
  const preps = useQuery({
    queryKey: [...prepsKey(jobId), archived],
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/interview-preps", { params: { path: { jobId }, query: { archived } } })).items,
    // Follow material the AI is still writing.
    refetchInterval: (q) => (q.state.data?.some((p) => p.status === "queued") ? 3000 : false),
  });
  const write = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/jobs/{jobId}/interview-preps", {
      params: { path: { jobId } }, body: focus.trim() ? { focus: focus.trim() } : {},
    })),
    onSuccess: async () => { setArchived(false); await client.invalidateQueries({ queryKey: prepsKey(jobId) }); },
  });
  // Open: the newest finished prep, and any newer one being written or failed
  // (so a failed retry never hides the last good material).
  const list = preps.data ?? [];
  const lastDone = list.findIndex((p) => p.status === "done");
  const shown = list.slice(0, lastDone + 1 || 1);
  const earlier = list.slice(shown.length);
  return (
    <section className="panel interview-prep" aria-label="Interview prep">
      <div className="row spread">
        <h3>Interview prep</h3>
        <div className="segmented" role="group" aria-label="Which prep">
          <button type="button" aria-pressed={!archived} className={archived ? undefined : "on"} onClick={() => { setArchived(false); }}>Current</button>
          <button type="button" aria-pressed={archived} className={archived ? "on" : undefined} onClick={() => { setArchived(true); }}>Archived</button>
        </div>
      </div>
      {canManage && (
        <form className="row prep-ask" onSubmit={(e) => { e.preventDefault(); write.mutate(); }}>
          <input aria-label="Focus (optional)" maxLength={1000} value={focus} onChange={(e) => { setFocus(e.target.value); }}
            placeholder="Focus (optional), e.g. strong technically, new to banking" />
          <button type="submit" className="primary" disabled={write.isPending}>{write.isPending ? "Asking the AI…" : "Write prep with AI"}</button>
        </form>
      )}
      <p className="muted small">From the job description, the client and the newest questions recorded above (up to 12, not archived). Salary is never sent to the AI.</p>
      <ErrorMessage error={write.error ?? preps.error} />
      {preps.data?.length === 0 && <p className="muted">{archived ? "No archived prep." : "No prep yet."}</p>}
      {shown.map((p) => <PrepCard key={p.id} jobId={jobId} jobTitle={jobTitle} prep={p} canManage={canManage} open />)}
      {earlier.length > 0 && (
        <details className="prep-earlier">
          <summary>Earlier prep ({earlier.length})</summary>
          {earlier.map((p) => <PrepCard key={p.id} jobId={jobId} jobTitle={jobTitle} prep={p} canManage={canManage} />)}
        </details>
      )}
    </section>
  );
}

function PrepCard({ jobId, jobTitle, prep, canManage, open = false }: { jobId: string; jobTitle: string; prep: Prep; canManage: boolean; open?: boolean }) {
  const client = useQueryClient();
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => { setCopied(false); }, 1500);
    return () => { clearTimeout(t); };
  }, [copied]);
  const archive = useMutation({
    mutationFn: async () => unwrap(await api.PATCH("/v1/interview-preps/{prepId}", { params: { path: { prepId: prep.id } }, body: { archived: !prep.archived } })),
    onSuccess: () => client.invalidateQueries({ queryKey: prepsKey(jobId) }),
  });
  const [exportError, setExportError] = useState<unknown>(null);
  const exportAs = (format: keyof typeof FILE_TYPES) => {
    setExportError(null);
    downloadFrom(`/v1/interview-preps/${prep.id}/export?format=${format}`, `Interview prep - ${jobTitle}.${format}`, FILE_TYPES[format])
      .catch((e: unknown) => { setExportError(e); });
  };
  const c = prep.content;
  const head = (
    <span className="prep-head">
      <strong>{formatWhen(prep.createdAt)}</strong>
      <span className="muted small">
        {prep.questionCount ? ` · from ${String(prep.questionCount)} recorded question${prep.questionCount === 1 ? "" : "s"}` : ""}
        {prep.focus ? ` · focus: ${prep.focus}` : ""}
      </span>
    </span>
  );
  return (
    <details className={`prep-card ${prep.status}`} open={open || undefined}>
      <summary>{head}</summary>
      {prep.status === "queued" && <p className="muted" role="status">The AI is writing it… this takes a minute.</p>}
      {prep.status === "failed" && <p className="error">{failure(prep.error)}</p>}
      {c && (
        <div className="prep-body">
          <div className="row prep-tools">
            <span className="prep-export" role="group" aria-label="Export">
              <span className="muted small">Export</span>
              <button type="button" onClick={() => { exportAs("docx"); }}>Word</button>
              <button type="button" onClick={() => { exportAs("pdf"); }}>PDF</button>
            </span>
            <button type="button" onClick={() => { void copyText(prepText(jobTitle, c)).then((ok) => { if (ok) setCopied(true); }); }}>
              {copied ? "Copied" : "Copy as text"}
            </button>
            {canManage && <IconButton label={prep.archived ? "Bring back this prep" : "Archive this prep"} tip={prep.archived ? "Bring back" : "Archive"}
              icon={prep.archived ? <RefreshIcon /> : <ArchiveIcon />} disabled={archive.isPending} onClick={() => { archive.mutate(); }} />}
          </div>
          {c.summary && <p className="prep-summary">{c.summary}</p>}
          {c.focusAreas.length > 0 && <p className="prep-focus">{c.focusAreas.map((f, i) => <span key={i} className="prep-chip">{f}</span>)}</p>}
          <PrepItems title="Soft skills" hint="situational, what-if and behavioural" items={c.softSkills} />
          <PrepItems title="Technical" items={c.technical} />
          {c.questionsToAsk.length > 0 && <>
            <h4>Questions to ask them</h4>
            <ul className="prep-asks">{c.questionsToAsk.map((q, i) => <li key={i}>{q}</li>)}</ul>
          </>}
        </div>
      )}
      {!c && canManage && prep.status !== "queued" && (
        <IconButton label={prep.archived ? "Bring back this prep" : "Archive this prep"} tip={prep.archived ? "Bring back" : "Archive"}
          icon={prep.archived ? <RefreshIcon /> : <ArchiveIcon />} disabled={archive.isPending} onClick={() => { archive.mutate(); }} />
      )}
      <ErrorMessage error={archive.error ?? exportError} />
    </details>
  );
}

/** Why the AI could not write it, saying once to try again. */
function failure(error: string | null): string {
  const why = (error ?? "The AI could not write it").trim().replace(/[.\s]+$/, "");
  return /try again/i.test(why) ? `${why}.` : `${why}. Try again.`;
}

function PrepItems({ title, hint, items }: { title: string; hint?: string; items: Item[] }) {
  const [all, setAll] = useState<boolean | null>(null); // null: as the person left each one
  if (items.length === 0) return null;
  return (
    <div className="prep-section">
      <div className="row spread">
        <h4>{title} {hint && <span className="muted small">({hint})</span>}</h4>
        <button type="button" className="link small" onClick={() => { setAll(!all); }}>{all ? "Collapse all" : "Expand all"}</button>
      </div>
      <ol className="prep-items">
        {items.map((it, i) => (
          <li key={i}>
            <details className="prep-item" open={all ?? i === 0}>
              <summary>
                <span className="prep-q">{it.question}</span>
                <span className="prep-badges">
                  <span className="prep-kind">{it.kind}</span>
                  {it.fromInterviews && <span className="prep-asked" title="Recorded at this job's interviews">Asked at interview</span>}
                </span>
              </summary>
              {it.whyAsked && <p className="prep-why"><span>Why they ask</span> {it.whyAsked}</p>}
              {it.approach.length > 0 && <div className="prep-block"><span>How to approach</span><ol>{it.approach.map((a, j) => <li key={j}>{a}</li>)}</ol></div>}
              {it.phrasing.length > 0 && (
                <div className="prep-block prep-say"><span>Say it like</span>{it.phrasing.map((p, j) => <blockquote key={j}>{p}</blockquote>)}</div>
              )}
              <div className="prep-pair">
                {it.tradeOffs.length > 0 && <div className="prep-block"><span>Trade-offs</span><ul>{it.tradeOffs.map((t, j) => <li key={j}>{t}</li>)}</ul></div>}
                {it.pitfalls.length > 0 && <div className="prep-block prep-avoid"><span>Avoid</span><ul>{it.pitfalls.map((t, j) => <li key={j}>{t}</li>)}</ul></div>}
              </div>
            </details>
          </li>
        ))}
      </ol>
    </div>
  );
}
