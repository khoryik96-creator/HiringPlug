// The questions asked at a job's interviews, as candidates and the client
// tell them: recorded here, kept with the job, archived once no longer asked
// (the AI's prep material uses the others).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { formatDay } from "../../labels";
import { ArchiveIcon, EditIcon, IconButton, RefreshIcon, TrashIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";
import { Field } from "../Field";
import { fieldErrors } from "../forms";
import "./interviewprep.css";

type Question = Schemas["InterviewQuestion"];
type Kind = Schemas["InterviewQuestionKind"];
type Round = Schemas["InterviewRound"];

export const interviewQuestionsKey = (jobId: string) => ["interview-questions", jobId] as const;

export const KIND: Record<Kind, string> = {
  technical: "Technical", situational: "Situational / what-if", behavioural: "Behavioural", culture: "Motivation & fit", other: "Other",
};
export const ROUND: Record<Round, string> = {
  screening: "Screening", hr: "HR", technical: "Technical", hiring_manager: "Hiring manager", panel: "Panel",
  case_study: "Case study", final: "Final", other: "Other",
};

type Draft = { question: string; kind: Kind; round: Round | ""; askedOn: string; source: string; notes: string };
const blank: Draft = { question: "", kind: "technical", round: "", askedOn: "", source: "", notes: "" };
const draftOf = (q: Question): Draft => ({
  question: q.question, kind: q.kind, round: q.round ?? "", askedOn: q.askedOn ?? "", source: q.source ?? "", notes: q.notes ?? "",
});

export function InterviewQuestions({ jobId, canManage }: { jobId: string; canManage: boolean }) {
  const [archived, setArchived] = useState(false);
  const [adding, setAdding] = useState(false);
  const list = useQuery({
    queryKey: [...interviewQuestionsKey(jobId), archived],
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/interview-questions", { params: { path: { jobId }, query: { archived } } })).items,
  });
  return (
    <section className="panel interview-questions" aria-label="Questions asked">
      <div className="row spread">
        <h3>Questions asked <span className="muted small">{list.data ? `(${String(list.data.length)})` : ""}</span></h3>
        <div className="row">
          <div className="segmented" role="group" aria-label="Which questions">
            <button type="button" aria-pressed={!archived} className={archived ? undefined : "on"} onClick={() => { setArchived(false); }}>In use</button>
            <button type="button" aria-pressed={archived} className={archived ? "on" : undefined} onClick={() => { setArchived(true); setAdding(false); }}>Archived</button>
          </div>
          {canManage && !archived && !adding && <button type="button" className="primary" onClick={() => { setAdding(true); }}>Record a question</button>}
        </div>
      </div>
      {adding && <QuestionForm jobId={jobId} onDone={() => { setAdding(false); }} />}
      <ErrorMessage error={list.error} />
      {list.data?.length === 0 && <p className="muted">{archived ? "No archived questions." : "No questions recorded yet: add what candidates tell you they were asked."}</p>}
      <ul className="iq-list">
        {list.data?.map((q) => <QuestionRow key={q.id} jobId={jobId} q={q} canManage={canManage} />)}
      </ul>
    </section>
  );
}

function QuestionRow({ jobId, q, canManage }: { jobId: string; q: Question; canManage: boolean }) {
  const client = useQueryClient();
  const [editing, setEditing] = useState(false);
  const change = useMutation({
    mutationFn: async (body: Schemas["InterviewQuestionPatch"]) => unwrap(await api.PATCH("/v1/interview-questions/{questionId}", {
      params: { path: { questionId: q.id } }, body,
    })),
    onSuccess: () => client.invalidateQueries({ queryKey: interviewQuestionsKey(jobId) }),
  });
  const remove = useMutation({
    mutationFn: async () => { unwrap(await api.DELETE("/v1/interview-questions/{questionId}", { params: { path: { questionId: q.id } } })); },
    onSuccess: () => client.invalidateQueries({ queryKey: interviewQuestionsKey(jobId) }),
  });
  if (editing) return <li className="iq-item"><QuestionForm jobId={jobId} question={q} onDone={() => { setEditing(false); }} /></li>;
  return (
    <li className="iq-item">
      <div className="iq-text">
        <strong>{q.question}</strong>
        <span className="iq-meta">
          <span className={`iq-chip ${q.kind}`}>{KIND[q.kind]}</span>
          {q.round && <span className="iq-chip">{ROUND[q.round]} round</span>}
          {q.askedOn && <span className="muted small">{formatDay(q.askedOn)}</span>}
          {q.source && <span className="muted small">· {q.source}</span>}
        </span>
        {q.notes && <span className="muted small iq-notes">{q.notes}</span>}
        <ErrorMessage error={change.error ?? remove.error} />
      </div>
      {canManage && (
        <span className="row iq-actions">
          {!q.archived && <IconButton label={`Edit "${q.question}"`} tip="Edit" icon={<EditIcon />} onClick={() => { setEditing(true); }} />}
          <IconButton label={q.archived ? `Bring back "${q.question}"` : `Archive "${q.question}"`} tip={q.archived ? "Bring back" : "Archive"}
            icon={q.archived ? <RefreshIcon /> : <ArchiveIcon />} disabled={change.isPending}
            onClick={() => { change.mutate({ version: q.version, archived: !q.archived }); }} />
          <IconButton label={`Delete "${q.question}"`} tip="Delete" icon={<TrashIcon />} danger disabled={remove.isPending}
            onClick={() => { if (window.confirm("Delete this question? Archive it instead to keep it.")) remove.mutate(); }} />
        </span>
      )}
    </li>
  );
}

function QuestionForm({ jobId, question, onDone }: { jobId: string; question?: Question; onDone: () => void }) {
  const client = useQueryClient();
  const [d, setD] = useState<Draft>(question ? draftOf(question) : blank);
  const set = (k: keyof Draft) => (e: { target: { value: string } }) => { setD({ ...d, [k]: e.target.value }); };
  const save = useMutation({
    mutationFn: async () => {
      const body = {
        question: d.question, kind: d.kind, round: d.round || null, askedOn: d.askedOn || null,
        source: d.source.trim() || null, notes: d.notes.trim() || null,
      };
      if (question) {
        return unwrap(await api.PATCH("/v1/interview-questions/{questionId}", { params: { path: { questionId: question.id } }, body: { ...body, version: question.version } }));
      }
      return unwrap(await api.POST("/v1/jobs/{jobId}/interview-questions", { params: { path: { jobId } }, body }));
    },
    onSuccess: async () => { await client.invalidateQueries({ queryKey: interviewQuestionsKey(jobId) }); onDone(); },
  });
  const errors = fieldErrors(save.error);
  return (
    <form className="iq-form form-grid" aria-label={question ? "Edit question" : "Record a question"} onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <div className="full">
        <Field label="Question" error={errors.question}>
          <textarea rows={2} required maxLength={2000} value={d.question} onChange={set("question")}
            placeholder="e.g. What would you do if the nightly batch ran late on month end?" />
        </Field>
      </div>
      <Field label="Kind">
        <select value={d.kind} onChange={set("kind")}>{(Object.keys(KIND) as Kind[]).map((k) => <option key={k} value={k}>{KIND[k]}</option>)}</select>
      </Field>
      <Field label="Round">
        <select value={d.round} onChange={set("round")}>
          <option value="">Not known</option>
          {(Object.keys(ROUND) as Round[]).map((r) => <option key={r} value={r}>{ROUND[r]}</option>)}
        </select>
      </Field>
      <Field label="Asked on" error={errors.askedOn}><input type="date" value={d.askedOn} onChange={set("askedOn")} /></Field>
      <Field label="Told by"><input maxLength={200} value={d.source} onChange={set("source")} placeholder="e.g. candidate after round 2" /></Field>
      <div className="full">
        <Field label="Notes"><input maxLength={2000} value={d.notes} onChange={set("notes")} placeholder="What the interviewer was after, how it went" /></Field>
      </div>
      <div className="row full">
        <button type="submit" className="primary" disabled={save.isPending || !d.question.trim()}>{question ? "Save" : "Save question"}</button>
        <button type="button" onClick={onDone}>Cancel</button>
      </div>
      <div className="full"><ErrorMessage error={save.error} /></div>
    </form>
  );
}
