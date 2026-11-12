// A candidate's screening calls, latest first (open), the others folded: when,
// by whom, what was said (the call's note, as questions and answers) and the
// salary given on that call. Shown at the top of Notes and activity.
import { useQuery } from "@tanstack/react-query";

import { api, type Permission, unwrap } from "../api/client";
import { notesKey, screeningCallsKey } from "../api/keys";
import { CallQA } from "./callLines";
import { ScreeningSalarySnapshot } from "./ScreeningSalarySnapshot";
import { formatDay } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

export function ScreeningCalls({ candidateId, permissions }: { candidateId: string; permissions: Permission[] }) {
  const calls = useQuery({
    queryKey: screeningCallsKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/screening-calls", { params: { path: { candidateId } } })).items,
  });
  const notes = useQuery({
    queryKey: notesKey("candidate", candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/notes", { params: { query: { subjectType: "candidate", subjectId: candidateId } } })),
    enabled: permissions.includes("notes:read"),
  });
  const noteOf = (id: string | null) => (id ? notes.data?.items.find((n) => n.id === id) : undefined);
  const list = calls.data ?? [];
  return (
    <section className="panel screening-calls" aria-label="Screening calls">
      <h3>Screening calls {list.length > 0 && <span className="muted small">({list.length})</span>}</h3>
      <ErrorMessage error={calls.error} />
      {calls.data && list.length === 0 && <p className="muted">No call logged yet.</p>}
      <ol className="call-list">
        {list.map((c, i) => {
          const note = noteOf(c.noteId);
          return (
            <li key={c.id} className={i === 0 ? "latest" : undefined} aria-label={i === 0 ? "Latest call" : undefined}>
              <details open={i === 0}>
                <summary>
                  <strong>{formatDay(c.calledOn)}</strong>
                  <span className="muted"> · {c.calledByName ?? "someone no longer here"}</span>
                  {i === 0 && <span className="badge interview">Latest</span>}
                  {c.salaryRecorded && <span className="badge placed">Salary updated</span>}
                  {note?.sensitive && <span className="muted small"> · 🔒 sensitive</span>}
                </summary>
                {note ? <CallQA body={note.body} />
                  : <p className="muted small">{c.noteId ? "The note is hidden for your role." : "No notes were written on this call."}</p>}
                {permissions.includes("candidates.compensation:read") && c.salarySnapshot && <ScreeningSalarySnapshot value={c.salarySnapshot} />}
              </details>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
