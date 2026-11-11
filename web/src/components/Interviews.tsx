import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { applicationInterviewsKey, interviewsKey } from "../api/keys";
import { INTERVIEW_MODE, INTERVIEW_STATUS, RECOMMENDATION, formatWhen } from "../labels";
import { EmailComposer } from "./EmailComposer";
import { ErrorMessage } from "./ErrorMessage";
import { InterviewForm } from "./InterviewForm";

type Interview = Schemas["Interview"];

/** An application's interviews, with scheduling for those who may. */
export function ApplicationInterviews({ applicationId, closed, permissions }: { applicationId: string; closed: boolean; permissions: Permission[] }) {
  const [scheduling, setScheduling] = useState(false);
  const list = useQuery({
    queryKey: applicationInterviewsKey(applicationId),
    queryFn: async () => unwrap(await api.GET("/v1/applications/{applicationId}/interviews", { params: { path: { applicationId } } })).items,
  });
  const canSchedule = permissions.includes("interviews:schedule");
  return (
    <div className="interviews">
      <ErrorMessage error={list.error} />
      {list.data?.map((iv) => <InterviewItem key={iv.id} interview={iv} permissions={permissions} />)}
      {canSchedule && !closed && !scheduling && (
        <button type="button" className="link" onClick={() => { setScheduling(true); }}>Schedule interview</button>
      )}
      {scheduling && <InterviewForm applicationId={applicationId} onDone={() => { setScheduling(false); }} />}
    </div>
  );
}

/** One interview: when, how, who, its feedback, and what the user may do with it. */
export function InterviewItem({ interview: iv, permissions, showCandidate = false }: { interview: Interview; permissions: Permission[]; showCandidate?: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [emailing, setEmailing] = useState(false);
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: interviewsKey });
    await queryClient.invalidateQueries({ queryKey: applicationInterviewsKey(iv.applicationId) });
  };
  const setStatus = useMutation({
    mutationFn: async (status: Interview["status"]) =>
      unwrap(await api.PATCH("/v1/interviews/{interviewId}", { params: { path: { interviewId: iv.id } }, body: { version: iv.version, status } })),
    onSettled: refresh,
  });
  const canSchedule = permissions.includes("interviews:schedule");
  const onPanel = iv.interviewers.some((p) => p.userId === me.data?.user.id);
  const mine = iv.feedback.find((f) => f.userId === me.data?.user.id);
  const link = iv.location && /^https?:\/\//.test(iv.location);
  if (editing) return <InterviewForm applicationId={iv.applicationId} interview={iv} onDone={() => { setEditing(false); }} />;
  return (
    <article className="interview" aria-label={`${iv.title} with ${iv.candidateName}`}>
      <div className="row spread">
        <div>
          <strong>{iv.title}</strong>{showCandidate && <> with <a href={`#/candidates/${iv.candidateId}`}>{iv.candidateName}</a> for {iv.jobTitle}</>}{" "}
          <span className={`badge ${iv.status}`}>{INTERVIEW_STATUS[iv.status]}</span>
          <div className="muted">
            {formatWhen(iv.startsAt)} · {iv.durationMinutes} min · {INTERVIEW_MODE[iv.mode]}
            {iv.location && <> · {link ? <a href={iv.location} target="_blank" rel="noreferrer">join</a> : iv.location}</>}
          </div>
          <div className="muted">With {iv.interviewers.map((p) => p.name).join(", ")}</div>
        </div>
        <div className="row">
          <button type="button" className="link" onClick={() => { void downloadFrom(`/v1/interviews/${iv.id}/calendar.ics`, "interview.ics", "text/calendar"); }}>
            Add to calendar
          </button>
          {canSchedule && iv.status === "scheduled" && (
            <>
              <button type="button" className="link" onClick={() => { setEditing(true); }}>Change</button>
              {permissions.includes("emails:send") && (
                <button type="button" className="link" onClick={() => { setEmailing(true); }}>Email candidate</button>
              )}
              <button type="button" className="link" onClick={() => { setStatus.mutate("completed"); }}>Done</button>
              <button type="button" className="link" onClick={() => { setStatus.mutate("no_show"); }}>Did not come</button>
              <button type="button" className="link" onClick={() => { setStatus.mutate("cancelled"); }}>Cancel</button>
            </>
          )}
        </div>
      </div>
      <ErrorMessage error={setStatus.error} />
      {emailing && (
        <EmailComposer permissions={permissions} candidateId={iv.candidateId} applicationId={iv.applicationId} interviewId={iv.id} template="Interview invitation"
          canManageTemplates={permissions.includes("emails.templates:manage")}
          canCreateTemplates={permissions.includes("emails.templates:create")} onDone={() => { setEmailing(false); }} />
      )}
      {iv.instructions && <p className="muted">For the candidate: {iv.instructions}</p>}
      {iv.feedback.map((f) => (
        <p key={f.userId}><strong>{f.name}:</strong> {RECOMMENDATION[f.recommendation]}{f.comments && <>: {f.comments}</>}</p>
      ))}
      {iv.feedbackHidden > 0 && <p className="muted">{iv.feedbackHidden} interviewer(s) gave feedback; you will see it once you give yours.</p>}
      {onPanel && permissions.includes("interviews:feedback") && iv.status !== "cancelled" && (iv.myFeedbackDue || mine) && (
        <FeedbackForm interview={iv} current={mine} onSaved={refresh} />
      )}
    </article>
  );
}

function FeedbackForm({ interview, current, onSaved }: { interview: Interview; current: Interview["feedback"][number] | undefined; onSaved: () => Promise<void> }) {
  const [open, setOpen] = useState(!current);
  const [recommendation, setRecommendation] = useState<Schemas["Recommendation"] | "">(current?.recommendation ?? "");
  const [comments, setComments] = useState(current?.comments ?? "");
  const save = useMutation({
    mutationFn: async () => {
      if (!recommendation) return;
      unwrap(await api.PUT("/v1/interviews/{interviewId}/feedback", {
        params: { path: { interviewId: interview.id } }, body: { recommendation, comments: comments.trim() || null },
      }));
    },
    onSuccess: async () => {
      await onSaved();
      setOpen(false);
    },
  });
  if (!open) return <button type="button" className="link" onClick={() => { setOpen(true); }}>Change my feedback</button>;
  return (
    <form className="feedback" aria-label="Your feedback" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <fieldset className="row">
        <legend>Your recommendation</legend>
        {(Object.keys(RECOMMENDATION) as Schemas["Recommendation"][]).map((r) => (
          <label key={r}><input type="radio" name={`rec-${interview.id}`} checked={recommendation === r} onChange={() => { setRecommendation(r); }} /> {RECOMMENDATION[r]}</label>
        ))}
      </fieldset>
      <textarea aria-label="Comments" rows={3} placeholder="Comments" value={comments} onChange={(e) => { setComments(e.target.value); }} />
      <button type="submit" disabled={!recommendation || save.isPending}>Save feedback</button>
      <ErrorMessage error={save.error} />
    </form>
  );
}

/** The dashboard's interview lists: what is coming up, and feedback still owed. */
export function MyInterviews({ permissions }: { permissions: Permission[] }) {
  const [from] = useState(() => new Date());
  const upcoming = useQuery({
    queryKey: [...interviewsKey, "upcoming"],
    queryFn: async () => unwrap(await api.GET("/v1/interviews", {
      params: { query: { mine: true, from: from.toISOString(), to: new Date(from.getTime() + 14 * 86_400_000).toISOString() } },
    })).items,
  });
  const due = useQuery({
    queryKey: [...interviewsKey, "feedback-due"],
    queryFn: async () => unwrap(await api.GET("/v1/interviews", { params: { query: { feedbackDue: true } } })).items,
  });
  return (
    <>
      <section className="panel" aria-label="My upcoming interviews">
        <h3>My upcoming interviews</h3>
        <ErrorMessage error={upcoming.error} />
        {upcoming.data?.length === 0 && <p className="muted">None in the next two weeks.</p>}
        {upcoming.data?.filter((iv) => iv.status === "scheduled").map((iv) => <InterviewItem key={iv.id} interview={iv} permissions={permissions} showCandidate />)}
      </section>
      {(due.data?.length ?? 0) > 0 && (
        <section className="panel" aria-label="Feedback to give">
          <h3>Feedback to give</h3>
          {due.data?.map((iv) => <InterviewItem key={iv.id} interview={iv} permissions={permissions} showCandidate />)}
        </section>
      )}
    </>
  );
}
