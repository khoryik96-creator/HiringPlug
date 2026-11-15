import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Schemas, unwrap } from "../api/client";
import { applicationInterviewsKey, interviewsKey, usersKey } from "../api/keys";
import { INTERVIEW_MODE, formatWhen } from "../labels";
import { SaveError } from "./SaveError";

type Interview = Schemas["Interview"];
type Clash = { title: string; startsAt: string; interviewers: string[] };

/** "2026-11-10T10:00" (a local date and time input) <-> an ISO instant. */
const toInput = (iso: string) => {
  const d = new Date(iso);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
};
const fromInput = (local: string) => new Date(local).toISOString();

/** Schedule an interview for an application, or change one. */
export function InterviewForm({ applicationId, interview, onDone }: { applicationId: string; interview?: Interview; onDone: () => void }) {
  const queryClient = useQueryClient();
  const users = useQuery({ queryKey: usersKey, queryFn: async () => unwrap(await api.GET("/v1/users")).items, staleTime: 60_000 });
  const [opened] = useState(() => ({ version: interview?.version ?? 0 }));
  const [form, setForm] = useState({
    title: interview?.title ?? "First interview",
    startsAt: interview ? toInput(interview.startsAt) : "",
    durationMinutes: interview?.durationMinutes ?? 60,
    mode: interview?.mode ?? "video",
    location: interview?.location ?? "",
    instructions: interview?.instructions ?? "",
    interviewerIds: interview?.interviewers.map((i) => i.userId) ?? [],
  });
  const save = useMutation({
    mutationFn: async (ignoreClashes: boolean) => {
      const body = {
        title: form.title, startsAt: fromInput(form.startsAt), durationMinutes: form.durationMinutes, mode: form.mode,
        location: form.location.trim() || null, instructions: form.instructions.trim() || null,
        interviewerIds: form.interviewerIds, ignoreClashes,
      };
      return interview
        ? unwrap(await api.PATCH("/v1/interviews/{interviewId}", { params: { path: { interviewId: interview.id } }, body: { version: opened.version, ...body } }))
        : unwrap(await api.POST("/v1/applications/{applicationId}/interviews", { params: { path: { applicationId } }, body }));
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: interviewsKey });
      await queryClient.invalidateQueries({ queryKey: applicationInterviewsKey(applicationId) });
      await queryClient.invalidateQueries({ queryKey: ["applications"] }); // the stage may have moved to interview
      onDone();
    },
  });
  const clashes = save.error instanceof ApiError && save.error.code === "INTERVIEW_CLASH" ? (save.error.details.clashes as Clash[]) : null;
  const toggle = (id: string) => {
    setForm({ ...form, interviewerIds: form.interviewerIds.includes(id) ? form.interviewerIds.filter((x) => x !== id) : [...form.interviewerIds, id] });
  };
  return (
    <form className="panel form-grid" aria-label={interview ? "Change interview" : "Schedule interview"}
      onSubmit={(e) => { e.preventDefault(); save.mutate(false); }}>
      <label className="field">Title<input required value={form.title} onChange={(e) => { setForm({ ...form, title: e.target.value }); }} /></label>
      <label className="field">Date and time<input type="datetime-local" required value={form.startsAt} onChange={(e) => { setForm({ ...form, startsAt: e.target.value }); }} /></label>
      <label className="field">Length
        <select value={form.durationMinutes} onChange={(e) => { setForm({ ...form, durationMinutes: Number(e.target.value) }); }}>
          {[15, 30, 45, 60, 90, 120, 180].map((m) => <option key={m} value={m}>{m} minutes</option>)}
        </select>
      </label>
      <label className="field">How
        <select value={form.mode} onChange={(e) => { setForm({ ...form, mode: e.target.value as Schemas["InterviewMode"] }); }}>
          {Object.entries(INTERVIEW_MODE).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      </label>
      <label className="field full">{form.mode === "video" ? "Meeting link" : form.mode === "phone" ? "Phone number or note" : "Address"}
        <input value={form.location} onChange={(e) => { setForm({ ...form, location: e.target.value }); }} />
      </label>
      <fieldset className="full">
        <legend>Interviewers</legend>
        {users.data?.filter((u) => u.canInterview || form.interviewerIds.includes(u.id)).map((u) => (
          <label key={u.id} className="row">
            <input type="checkbox" name="interviewers" required={form.interviewerIds.length === 0} checked={form.interviewerIds.includes(u.id)} onChange={() => { toggle(u.id); }} /> {u.name}
          </label>
        ))}
      </fieldset>
      <label className="field full">Instructions for the candidate
        <textarea rows={3} value={form.instructions} onChange={(e) => { setForm({ ...form, instructions: e.target.value }); }} />
      </label>
      {clashes ? (
        <div className="full warning" role="alert">
          <p>Already busy then:</p>
          <ul>{clashes.map((c, i) => <li key={i}>{c.interviewers.join(", ")}: {c.title}, {formatWhen(c.startsAt)}</li>)}</ul>
          <button type="button" onClick={() => { save.mutate(true); }}>Schedule anyway</button>
        </div>
      ) : (
        <SaveError error={save.error} shown={[]} onReload={onDone} />
      )}
      <div className="row full">
        <button type="submit" disabled={save.isPending}>
          {interview ? "Save" : "Schedule"}
        </button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}
