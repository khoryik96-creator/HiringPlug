import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { assistantAnswersKey, assistantKey, candidateApplicationsKey } from "../api/keys";
import { ASSISTANT_KIND, formatWhen } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type SubjectType = Schemas["AssistantSubjectType"];
type Kind = Schemas["AssistantKind"];
type Answer = Schemas["AssistantAnswer"];
type Subject = { type: SubjectType; id: string; label: string };

/**
 * The AI assistant on a candidate's or a job's page. On a candidate's page it
 * can also be asked about them for one of their jobs (how well they fit).
 * It sees only what the user may see, never contact details or salary.
 */
export function Assistant({ subjectType, subjectId, permissions }: { subjectType: "candidate" | "job"; subjectId: string; permissions: Permission[] }) {
  const allowed = permissions.includes("assistant:use");
  const status = useQuery({ queryKey: assistantKey, queryFn: async () => unwrap(await api.GET("/v1/assistant")), enabled: allowed });
  const applications = useQuery({
    queryKey: candidateApplicationsKey(subjectId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/applications", { params: { path: { candidateId: subjectId } } })),
    enabled: allowed && subjectType === "candidate" && permissions.includes("pipeline:read"),
  });
  const own: Subject = { type: subjectType, id: subjectId, label: subjectType === "candidate" ? "This candidate" : "This job" };
  const subjects: Subject[] = [own, ...(subjectType === "candidate" ? applications.data?.items ?? [] : []).map(
    (a): Subject => ({ type: "application", id: a.id, label: `For ${a.jobTitle} (${a.companyName})` }),
  )];
  const [chosen, setChosen] = useState(own.id);
  const subject = subjects.find((s) => s.id === chosen) ?? own;

  if (!allowed) return null;
  const s = status.data;
  return (
    <section className="panel" aria-label="AI assistant">
      <h3>AI assistant</h3>
      <ErrorMessage error={status.error} />
      {s && !s.enabled && (
        <p className="muted">Off: no AI provider is set up.</p>
      )}
      {s?.enabled && (
        <>
          {subjects.length > 1 && (
            <label>About{" "}
              <select value={subject.id} onChange={(e) => { setChosen(e.target.value); }}>
                {subjects.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
              </select>
            </label>
          )}
          <Conversation key={subject.id} subject={subject} />
          <p className="muted small">
            {s.usedTokens.toLocaleString("en")} of {s.monthlyTokens.toLocaleString("en")} tokens this month
          </p>
        </>
      )}
    </section>
  );
}

/** Asking about one subject, and the answers so far (yours only). */
function Conversation({ subject }: { subject: Subject }) {
  const queryClient = useQueryClient();
  const [question, setQuestion] = useState("");
  const key = assistantAnswersKey(subject.type, subject.id);
  const answers = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/assistant/answers", { params: { query: { subjectType: subject.type, subjectId: subject.id } } })).items,
    refetchInterval: (q) => (q.state.data?.some((a) => a.status === "queued") ? 2000 : false),
  });
  const ask = useMutation({
    mutationFn: async ({ kind, text }: { kind: Kind; text?: string }) => unwrap(await api.POST("/v1/assistant/answers", {
      body: { subjectType: subject.type, subjectId: subject.id, kind, ...(text ? { question: text } : {}) },
    })),
    onSuccess: async (_, { kind }) => {
      if (kind === "question") setQuestion("");
      await queryClient.invalidateQueries({ queryKey: key });
      await queryClient.invalidateQueries({ queryKey: assistantKey, exact: true });
    },
  });
  const kinds: Kind[] = subject.type === "application" ? ["fit", "red_flags"] : ["summary", "red_flags"];
  const waiting = ask.isPending || (answers.data?.some((a) => a.status === "queued") ?? false);
  return (
    <div className="assistant">
      <div className="row">
        {kinds.map((k) => (
          <button key={k} type="button" disabled={waiting} onClick={() => { ask.mutate({ kind: k }); }}>{ASSISTANT_KIND[k]}</button>
        ))}
      </div>
      <form className="row" onSubmit={(e) => { e.preventDefault(); if (question.trim()) ask.mutate({ kind: "question", text: question.trim() }); }}>
        <input aria-label="Ask a question" placeholder="Ask a question" maxLength={1000}
          value={question} onChange={(e) => { setQuestion(e.target.value); }} />
        <button type="submit" disabled={waiting || !question.trim()}>Ask</button>
      </form>
      <ErrorMessage error={ask.error ?? answers.error} />
      {answers.data?.map((a) => <AnswerItem key={a.id} a={a} />)}
    </div>
  );
}

function AnswerItem({ a }: { a: Answer }) {
  return (
    <article className="answer" aria-label={a.question ?? ASSISTANT_KIND[a.kind]}>
      <strong>{a.question ?? ASSISTANT_KIND[a.kind]}</strong>
      {a.status === "queued" && <p className="muted" role="status">Thinking…</p>}
      {a.status === "failed" && <p className="warning">No answer: {a.error}</p>}
      {a.status === "done" && <p className="pre">{a.answer}</p>}
      <div className="muted small">{formatWhen(a.createdAt)}{a.model && <> · {a.model}</>}</div>
    </article>
  );
}
