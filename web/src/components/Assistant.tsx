import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { createPortal } from "react-dom";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { assistantAnswersKey, assistantKey, candidateApplicationsKey } from "../api/keys";
import { ASSISTANT_KIND, formatWhen } from "../labels";
import { AdText } from "./AdText";
import { ErrorMessage } from "./ErrorMessage";
import { type DockState, useDock } from "./useDock";

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

  const dock = useDock("assistant");
  if (!allowed) return null;
  const s = status.data;
  const body = (
    <>
      <ErrorMessage error={status.error} />
      {s && !s.enabled && (
        <p className="muted">Off: no AI provider is set up.{permissions.includes("aiproviders:manage") && <> <a href="#/ai-provider">Set one up</a></>}</p>
      )}
      {s?.enabled && (
        <>
          {subjects.length > 1 && (
            <label className="assistant-about">About
              <select value={subject.id} onChange={(e) => { setChosen(e.target.value); }}>
                {subjects.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
              </select>
            </label>
          )}
          <Conversation key={subject.id} subject={subject} />
          <p className="muted small assistant-usage">
            {s.usedTokens.toLocaleString("en")} of {s.monthlyTokens.toLocaleString("en")} tokens this month
          </p>
        </>
      )}
    </>
  );
  return <Docked state={dock.state} onState={dock.setState} width={dock.width} onWidth={dock.setWidth}>{body}</Docked>;
}

/**
 * The assistant in the page, docked to the right of the screen (the page
 * makes room; drag its edge to resize), or tucked away as a tab.
 */
function Docked({ state, onState, width, onWidth, children }: {
  state: DockState; onState: (s: DockState) => void; width: number; onWidth: (w: number) => void; children: ReactNode;
}) {
  const head = (
    <header className="assistant-head">
      <h3>AI assistant</h3>
      <span className="assistant-tools">
        {state === "inline" ? (
          <button type="button" className="tool-btn" onClick={() => { onState("right"); }}>Dock right ⇥</button>
        ) : (
          <>
            <button type="button" className="tool-btn" aria-label="Minimise the AI assistant" title="Minimise" onClick={() => { onState("min"); }}>–</button>
            <button type="button" className="tool-btn" onClick={() => { onState("inline"); }}>⇤ Back in the page</button>
          </>
        )}
      </span>
    </header>
  );
  if (state === "inline") {
    return <section className="panel assistant-panel" aria-label="AI assistant">{head}{children}</section>;
  }
  const note = (
    <p className="assistant-note muted small">
      The AI assistant is {state === "right" ? "docked on the right" : "tucked away on the right edge"}.{" "}
      <button type="button" className="link" onClick={() => { onState("inline"); }}>Show it here</button>
    </p>
  );
  if (state === "min") {
    return (
      <>
        {note}
        {createPortal(
          <button type="button" className="assistant-tab" onClick={() => { onState("right"); }}>AI assistant</button>,
          document.body,
        )}
      </>
    );
  }
  const resize = (e: React.PointerEvent) => {
    e.preventDefault();
    const move = (ev: PointerEvent) => { onWidth(window.innerWidth - ev.clientX); };
    const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };
  return (
    <>
      {note}
      {createPortal(
        <aside className="assistant-dock" aria-label="AI assistant" style={{ width }}>
          <div className="dock-handle" role="separator" aria-orientation="vertical" aria-label="Drag to resize" onPointerDown={resize} />
          {head}
          <div className="assistant-scroll">{children}</div>
        </aside>,
        document.body,
      )}
    </>
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
      <div className="assistant-quick">
        {kinds.map((k) => (
          <button key={k} type="button" className="chip-btn" disabled={waiting} onClick={() => { ask.mutate({ kind: k }); }}>{ASSISTANT_KIND[k]}</button>
        ))}
      </div>
      <form className="assistant-ask" onSubmit={(e) => { e.preventDefault(); if (question.trim()) ask.mutate({ kind: "question", text: question.trim() }); }}>
        <input aria-label="Ask a question" placeholder="Ask a question" maxLength={1000}
          value={question} onChange={(e) => { setQuestion(e.target.value); }} />
        <button type="submit" disabled={waiting || !question.trim()}>Ask</button>
      </form>
      <ErrorMessage error={ask.error ?? answers.error} />
      {answers.data?.length === 0 && <p className="muted small">Ask for a summary, the gaps and red flags, or anything about them.</p>}
      <div className="answers">{answers.data?.map((a, i) => <AnswerItem key={a.id} a={a} startOpen={i === 0} />)}</div>
    </div>
  );
}

/** Long answers past the newest are folded to a few lines. */
const LONG = 500;

function AnswerItem({ a, startOpen }: { a: Answer; startOpen: boolean }) {
  const long = (a.answer?.length ?? 0) > LONG;
  const [open, setOpen] = useState(startOpen || !long);
  const title = a.question ?? ASSISTANT_KIND[a.kind];
  return (
    <article className={`answer${open ? "" : " folded"}`} aria-label={title}>
      <header className="answer-head">
        <strong>{title}</strong>
        <span className="muted small">{formatWhen(a.createdAt)}</span>
      </header>
      {a.status === "queued" && <p className="muted thinking" role="status">Thinking…</p>}
      {a.status === "failed" && <p className="warning">No answer: {a.error}</p>}
      {a.status === "done" && <AdText text={a.answer ?? ""} />}
      <footer className="answer-foot muted small">
        {a.model && <span>{a.model}</span>}
        {long && a.status === "done" && (
          <button type="button" className="link" onClick={() => { setOpen(!open); }}>{open ? "Show less" : "Show all"}</button>
        )}
      </footer>
    </article>
  );
}
