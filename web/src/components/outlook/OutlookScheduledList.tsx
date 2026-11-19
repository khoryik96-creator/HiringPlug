import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

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
  const [pages, setPages] = useState([0]);
  const offset = pages.at(-1) ?? 0;
  const list = useQuery({
    queryKey: [...outlookScheduledKey, offset],
    queryFn: async () => unwrap(await api.GET("/v1/outlook/scheduled", { params: { query: { offset } } })),
    refetchInterval: (q) => q.state.data?.items.some((item) => item.status === "scheduled" || item.status === "confirming") ? 5000 : false,
  });
  const action = useMutation({
    mutationFn: async ({ item, resolve }: { item: Send; resolve?: boolean }) => unwrap(await api.POST(
      resolve ? "/v1/outlook/scheduled/{sendId}/resolve" : "/v1/outlook/scheduled/{sendId}/cancel", {
      params: { path: { sendId: item.id } }, body: { version: item.version },
    })),
    onSuccess: async () => { await client.invalidateQueries({ queryKey: outlookScheduledKey }); },
  });
  return <section className="panel outlook-queue" aria-label="Scheduled email">
    <ErrorMessage error={list.error ?? action.error}/>
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
        <button type="button" disabled={action.isPending} onClick={() => { action.mutate({ item }); }}>Cancel send</button>
      </div>}
      {item.status === "uncertain" && <button type="button" disabled={action.isPending}
        onClick={() => { action.mutate({ item, resolve: true }); }}>Check Outlook again</button>}
    </article>)}
    {(offset > 0 || list.data?.nextOffset != null) && <div className="row">
      {offset > 0 && <button type="button" onClick={() => { setPages(pages.slice(0, -1)); action.reset(); }}>Previous</button>}
      {list.data?.nextOffset != null && <button type="button" onClick={() => {
        const next = list.data?.nextOffset;
        if (next != null) setPages([...pages, next]);
        action.reset();
      }}>Next</button>}
    </div>}
  </section>;
}
