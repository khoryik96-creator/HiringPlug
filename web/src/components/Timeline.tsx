import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { activityKey, notesKey } from "../api/keys";
import { keysOf, NOTE_KIND } from "../labels";
import { describe } from "./activityText";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { fieldErrors } from "./forms";

type Subject = Schemas["NoteSubjectType"];

/** Questions to start a screening note with; edit them freely. */
export const SCREENING_TEMPLATE = [
  "Current role and reason for leaving:",
  "Notice period and availability:",
  "Current and expected salary:",
  "Work authorization:",
  "Key skills for this role:",
  "Concerns:",
].join("\n");

const when = (iso: string) => new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });

/** Notes on a record and what changed on it, newest first, with a note form. */
export function Timeline({ subjectType, subjectId, permissions }: { subjectType: Subject; subjectId: string; permissions: Permission[] }) {
  const canWrite = permissions.includes("notes:create");
  const canSensitive = permissions.includes("notes.sensitive:read");
  const notes = useQuery({
    queryKey: notesKey(subjectType, subjectId),
    queryFn: async () => unwrap(await api.GET("/v1/notes", { params: { query: { subjectType, subjectId } } })),
    enabled: permissions.includes("notes:read"),
  });
  const activity = useQuery({
    queryKey: activityKey(subjectType, subjectId),
    queryFn: async () => unwrap(await api.GET("/v1/activity", { params: { query: { entityType: subjectType, entityId: subjectId } } })),
    enabled: permissions.includes("notes.activity:read"),
  });

  const items = [
    ...(notes.data?.items ?? []).map((n) => ({ key: n.id, at: n.createdAt, note: n, text: null as string | null, who: n.authorName })),
    ...(activity.data?.items ?? []).flatMap((a) => {
      const text = describe(a);
      return text ? [{ key: a.id, at: a.at, note: null, text, who: a.actorName }] : [];
    }),
  ].sort((x, y) => y.at.localeCompare(x.at));

  return (
    <section className="panel" aria-label="Notes and activity">
      <h3>Notes and activity</h3>
      {canWrite && <NoteForm subjectType={subjectType} subjectId={subjectId} canSensitive={canSensitive} />}
      <ErrorMessage error={notes.error ?? activity.error} />
      {notes.data && notes.data.hiddenSensitive > 0 && (
        <p className="muted">🔒 {String(notes.data.hiddenSensitive)} sensitive note{notes.data.hiddenSensitive === 1 ? " is" : "s are"} hidden for your role.</p>
      )}
      <ul className="timeline">
        {items.map((i) => (
          <li key={i.key} className={i.note ? "note" : "change"}>
            <div className="muted">
              {i.who} · {when(i.at)}
              {i.note && <> · {NOTE_KIND[i.note.kind]}{i.note.sensitive && " · 🔒 sensitive"}</>}
            </div>
            {i.note ? <p className="pre">{i.note.body}</p> : <p>{i.who} {i.text}</p>}
          </li>
        ))}
        {items.length === 0 && <li className="muted">Nothing yet.</li>}
      </ul>
    </section>
  );
}

function NoteForm({ subjectType, subjectId, canSensitive }: { subjectType: Subject; subjectId: string; canSensitive: boolean }) {
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<Schemas["NoteKind"]>("general");
  const [body, setBody] = useState("");
  const [sensitive, setSensitive] = useState(false);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/notes", { body: { subjectType, subjectId, kind, body, sensitive } })),
    onSuccess: () => {
      setBody("");
      setSensitive(false);
      void queryClient.invalidateQueries({ queryKey: notesKey(subjectType, subjectId) });
    },
  });
  const errors = fieldErrors(save.error);
  return (
    <form className="form-grid" aria-label="Add a note" onSubmit={(e) => { e.preventDefault(); if (body.trim()) save.mutate(); }}>
      <Field label="Kind">
        <select value={kind} onChange={(e) => {
          const k = e.target.value as Schemas["NoteKind"];
          setKind(k);
          if (k === "screening" && body.trim() === "") setBody(SCREENING_TEMPLATE);
        }}>
          {keysOf(NOTE_KIND).map((k) => <option key={k} value={k}>{NOTE_KIND[k]}</option>)}
        </select>
      </Field>
      <div className="full">
        <Field label="Note" error={errors.body}>
          <textarea rows={kind === "screening" ? 7 : 3} value={body} onChange={(e) => { setBody(e.target.value); }} />
        </Field>
      </div>
      <div className="row full">
        <button type="submit" disabled={save.isPending || body.trim() === ""}>Add note</button>
        {canSensitive && (
          <label className="row">
            <input type="checkbox" checked={sensitive} onChange={(e) => { setSensitive(e.target.checked); }} /> Sensitive (only roles that see salaries)
          </label>
        )}
      </div>
      {Object.keys(errors).length === 0 && <div className="full"><ErrorMessage error={save.error} /></div>}
    </form>
  );
}
