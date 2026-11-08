import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { activityKey, candidateKey, notesKey } from "../api/keys";
import { CallItems, type CallAnswers, callNote, useCallItems } from "../components/CallItems";
import { ErrorMessage } from "../components/ErrorMessage";
import { Field } from "../components/Field";

/** A reference check: the questions set in Settings > Reference check, saved as a note on the candidate. */
export function ReferenceCheck({ candidateId, permissions }: { candidateId: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const candidate = useQuery({
    queryKey: candidateKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}", { params: { path: { candidateId } } })),
  });
  const items = useCallItems("reference_check");
  const [answers, setAnswers] = useState<CallAnswers>({});
  const [other, setOther] = useState("");
  const [sensitive, setSensitive] = useState(false);
  const [saved, setSaved] = useState(false);
  const body = callNote(items, answers, other);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/notes", {
      body: { subjectType: "candidate", subjectId: candidateId, kind: "reference", body, sensitive },
    })),
    onSuccess: async () => {
      setAnswers({});
      setOther("");
      setSensitive(false);
      setSaved(true);
      await queryClient.invalidateQueries({ queryKey: notesKey("candidate", candidateId) });
      await queryClient.invalidateQueries({ queryKey: activityKey("candidate", candidateId) });
    },
  });
  const c = candidate.data;
  if (!c) return candidate.error ? <ErrorMessage error={candidate.error} /> : <p className="muted">Loading…</p>;
  return (
    <main className="roomy">
      <a href={`#/candidates/${candidateId}`}>← {c.fullName}</a>
      <h2>Reference check: {c.fullName}</h2>
      {!permissions.includes("notes:create") ? <p className="muted">Your role may not write notes.</p> : (
        <form className="panel form-grid" aria-label="Reference check" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
          <p className="muted small full">
            What to ask is set in Settings &gt; Reference check. The answers are saved as a reference check note on their page,
            each after its question. Several referees: save one, then the next.
          </p>
          <CallItems list="reference_check" answers={answers} onChange={(a) => { setSaved(false); setAnswers(a); }} />
          <Field label="Other notes">
            <textarea rows={4} value={other} onChange={(e) => { setSaved(false); setOther(e.target.value); }} />
          </Field>
          {permissions.includes("notes.sensitive:read") && (
            <label className="row full"><input type="checkbox" checked={sensitive} onChange={(e) => { setSensitive(e.target.checked); }} /> Sensitive note (only roles that see sensitive notes)</label>
          )}
          <div className="row full">
            <button type="submit" disabled={save.isPending || !body}>Save the reference check</button>
            {saved && <span role="status" className="muted">Saved to {c.fullName}&apos;s notes.</span>}
          </div>
          <ErrorMessage error={save.error} />
        </form>
      )}
    </main>
  );
}
