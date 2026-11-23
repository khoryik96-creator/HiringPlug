// Formatting several candidates' CVs at once, CV Studio's way, with the same
// options: each one's newest CV they sent (PDF or Word). Those that cannot be
// formatted now are listed with the reason; the others are made in the
// background and appear on each candidate's page.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { loadSummaryChoice, saveSummaryChoice, studioCvsKey, SummaryOptions } from "./FormatCv";
import { fieldErrors } from "./forms";

/** At most this many in one go (the API's limit). */
export const BATCH_MAX = 20;

export type BatchPerson = { candidateId: string; name: string };

export function BatchFormat({ people, onClose }: { people: BatchPerson[]; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ blind: false, neutralGender: false, correctLanguage: true, alignment: "left" as "left" | "justify" });
  const [summary, setSummary] = useState(loadSummaryChoice);
  const start = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/studio-cvs/batch", {
      body: {
        candidateIds: people.map((p) => p.candidateId),
        blind: form.blind, neutralGender: form.blind && form.neutralGender, correctLanguage: form.correctLanguage, alignment: form.alignment,
        summary: summary.write ? summary.length : "none",
        ...(summary.write ? { summaryFocus: summary.focus, ...(summary.direction.trim() ? { summaryDirection: summary.direction.trim() } : {}) } : {}),
      },
    })),
    onSuccess: async (out) => {
      saveSummaryChoice(summary);
      await Promise.all(out.items.filter((i) => i.started).map((i) => queryClient.invalidateQueries({ queryKey: studioCvsKey(i.candidateId) })));
    },
  });
  const nameOf = (id: string) => people.find((p) => p.candidateId === id)?.name ?? "A candidate";

  if (start.data) return <BatchResult items={start.data.items} nameOf={nameOf} onClose={onClose} />;
  const tooMany = people.length > BATCH_MAX;
  return (
    <section className="panel batch-format" aria-label="Format CVs">
      <div className="row spread">
        <h3>Format {people.length} CV{people.length === 1 ? "" : "s"} CV Studio&apos;s way</h3>
        <button type="button" onClick={onClose}>Cancel</button>
      </div>
      <p className="muted small">
        Each candidate&apos;s newest CV they sent (PDF or Word) is formatted with these options, in the background. Anyone who cannot be
        formatted now (no CV, one being made already, no room for another formatted CV) is listed afterwards with the reason.
      </p>
      <p className="small batch-names">{people.map((p) => p.name).join(", ")}</p>
      <form className="form-grid" onSubmit={(e) => { e.preventDefault(); if (!tooMany) start.mutate(); }}>
        <label className="row full">
          <input type="checkbox" checked={form.correctLanguage} onChange={(e) => { setForm({ ...form, correctLanguage: e.target.checked }); }} />{" "}
          Correct spelling, punctuation and capitals (never the meaning, numbers or dates)
        </label>
        <label className="row full">
          <input type="checkbox" checked={form.blind} onChange={(e) => { setForm({ ...form, blind: e.target.checked }); }} />{" "}
          Blind CVs (no name or contact details; employers and schools described, not named)
        </label>
        {form.blind && (
          <label className="row full">
            <input type="checkbox" checked={form.neutralGender} onChange={(e) => { setForm({ ...form, neutralGender: e.target.checked }); }} />{" "}
            &ldquo;The candidate&rdquo; instead of he or she
          </label>
        )}
        <SummaryOptions choice={summary} onChange={setSummary} error={fieldErrors(start.error).summaryDirection} />
        <label className="field">Paragraphs
          <select value={form.alignment} onChange={(e) => { setForm({ ...form, alignment: e.target.value === "justify" ? "justify" : "left" }); }}>
            <option value="left">Aligned left</option>
            <option value="justify">Justified</option>
          </select>
        </label>
        <ErrorMessage error={fieldErrors(start.error).summaryDirection ? null : start.error} />
        <div className="row full">
          <button type="submit" className="primary" disabled={start.isPending || tooMany}>
            {start.isPending ? "Starting…" : `Format ${people.length} CV${people.length === 1 ? "" : "s"}`}
          </button>
          {tooMany && <span className="warning small">At most {BATCH_MAX} at a time.</span>}
        </div>
      </form>
    </section>
  );
}

function BatchResult({ items, nameOf, onClose }: { items: Schemas["StudioCvBatchItem"][]; nameOf: (id: string) => string; onClose: () => void }) {
  const started = items.filter((i) => i.started);
  const skipped = items.filter((i) => !i.started);
  return (
    <section className="panel batch-format" aria-label="CVs being formatted">
      <h3>{started.length} CV{started.length === 1 ? " is" : "s are"} being formatted</h3>
      {started.length > 0 && (
        <p className="muted small">
          They are made one after another in the background; each appears under CVs on the candidate&apos;s page when ready.
          {" "}{started.map((i) => nameOf(i.candidateId)).join(", ")}.
        </p>
      )}
      {skipped.length > 0 && (
        <>
          <p><strong>Not formatted ({skipped.length}):</strong></p>
          <ul className="batch-skipped">
            {skipped.map((i) => (
              <li key={i.candidateId}>
                <a href={`#/candidates/${i.candidateId}`}>{nameOf(i.candidateId)}</a>: {i.reason}
              </li>
            ))}
          </ul>
        </>
      )}
      <button type="button" onClick={onClose}>Done</button>
    </section>
  );
}
