import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { formatWhen } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { outlookScheduledKey } from "./OutlookDraftEditor";

type Send = Schemas["OutlookScheduledSend"];
const labels: Record<Send["status"], string> = {
  scheduled: "Scheduled", confirming: "Confirming in Outlook", sent: "Sent",
  needs_review: "Needs review", uncertain: "Check Outlook", cancelled: "Cancelled",
};

export function OutlookScheduledList({ onOpen }: { onOpen: (id: string) => void }) {
  const client = useQueryClient();
  const list = useQuery({
    queryKey: outlookScheduledKey,
    queryFn: async () => unwrap(await api.GET("/v1/outlook/scheduled")),
    refetchInterval: (q) => q.state.data?.items.some((item) => item.status === "scheduled" || item.status === "confirming") ? 5000 : false,
  });
  const cancel = useMutation({
    mutationFn: async (item: Send) => unwrap(await api.POST("/v1/outlook/scheduled/{sendId}/cancel", {
      params: { path: { sendId: item.id } }, body: { version: item.version },
    })),
    onSuccess: async () => { await client.invalidateQueries({ queryKey: outlookScheduledKey }); },
  });
  return <section className="panel outlook-queue" aria-label="Scheduled email">
    <ErrorMessage error={list.error ?? cancel.error}/>
    {list.isPending && <p className="muted">Loading scheduled email…</p>}
    {list.data?.items.length === 0 && <p className="muted">No scheduled email yet.</p>}
    {list.data?.items.map((item) => <article key={item.id} className="outlook-send-row">
      <div>
        <strong>{item.subject || "(No subject)"}</strong>
        <p className="small muted">{formatWhen(item.sendAt)} · {labels[item.status]}</p>
        {item.error && <p className="small warning">{item.error}</p>}
      </div>
      {(item.status === "scheduled" || item.status === "needs_review") && <div className="row outlook-actions">
        <button type="button" onClick={() => { onOpen(item.messageId); }}>Open draft</button>
        <button type="button" disabled={cancel.isPending} onClick={() => { cancel.mutate(item); }}>Cancel send</button>
      </div>}
    </article>)}
  </section>;
}
