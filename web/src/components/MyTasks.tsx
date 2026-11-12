// The dashboard's tasks: what automations gave you to do, ticked off here.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";
import { formatDay } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { todayISO } from "./placementTimeline";

const tasksKey = ["tasks"] as const;

export function MyTasks({ approvals }: { approvals: boolean }) {
  const queryClient = useQueryClient();
  const tasks = useQuery({ queryKey: tasksKey, queryFn: async () => unwrap(await api.GET("/v1/tasks")).items });
  const waiting = useQuery({
    queryKey: ["automation-runs", "to_approve"],
    queryFn: async () => unwrap(await api.GET("/v1/automation-runs", { params: { query: { status: "to_approve" } } })).items,
    enabled: approvals,
  });
  const tick = useMutation({
    mutationFn: async ({ id, done }: { id: string; done: boolean }) =>
      unwrap(await api.PATCH("/v1/tasks/{taskId}", { params: { path: { taskId: id } }, body: { done } })),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tasksKey }),
  });
  const items = tasks.data ?? [];
  const toApprove = waiting.data?.length ?? 0;
  if (items.length === 0 && toApprove === 0) return null;
  const today = todayISO();
  return (
    <section className="panel" aria-label="Your tasks">
      <div className="row spread">
        <h3>Your tasks</h3>
        {toApprove > 0 && <a href="#/automations">{toApprove} email{toApprove === 1 ? "" : "s"} to approve</a>}
      </div>
      <ErrorMessage error={tasks.error ?? tick.error} />
      <ul className="task-list">
        {items.map((t) => (
          <li key={t.id} className={t.doneAt ? "done" : ""}>
            <label className="check">
              <input type="checkbox" checked={t.doneAt !== null} disabled={tick.isPending} onChange={(e) => { tick.mutate({ id: t.id, done: e.target.checked }); }} />
              {" "}{t.title}
            </label>
            <span className={`muted small${!t.doneAt && t.dueOn < today ? " late" : ""}`}>
              {t.candidateId && <><a href={`#/candidates/${t.candidateId}`}>{t.candidateName}</a> · </>}{formatDay(t.dueOn)}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
