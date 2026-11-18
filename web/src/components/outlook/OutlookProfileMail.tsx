import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";

export type OutlookProfile = { candidateId: string; contactId?: never } | { contactId: string; candidateId?: never };

export function outlookHref(profile?: OutlookProfile, address?: string | null, conversation?: string) {
  const params = new URLSearchParams();
  if (profile?.candidateId) params.set("candidateId", profile.candidateId);
  if (profile?.contactId) params.set("contactId", profile.contactId);
  if (address) params.set("search", address);
  if (conversation) params.set("conversation", conversation);
  return `#/outlook?${params.toString()}`;
}

export function OutlookProfileMail({ profile, address }: { profile: OutlookProfile; address?: string | null }) {
  const [open, setOpen] = useState(false);
  const query = useQuery({
    queryKey: ["outlook", "profile", profile],
    queryFn: async () => unwrap(await api.GET("/v1/outlook/mail", { params: { query: profile } })),
    enabled: open,
  });
  const seen = new Set<string>();
  const threads = query.data?.items.filter((item) => {
    if (seen.has(item.conversation)) return false;
    seen.add(item.conversation);
    return true;
  });
  return <details onToggle={(e) => { setOpen(e.currentTarget.open); }}>
    <summary>My Outlook conversations</summary>
    <ErrorMessage error={query.error}/>
    {query.isFetching && <p className="muted">Loading Outlook conversations…</p>}
    {threads?.length === 0 && <p className="muted">No linked conversations.</p>}
    {threads && threads.length > 0 && <ul>{threads.map((thread) =>
      <li key={thread.conversation}><a href={outlookHref(profile, undefined, thread.conversation)}>{thread.subject || "(No subject)"}</a></li>)}</ul>}
    <div className="row">
      <a href={outlookHref(profile, address)}>Find in Outlook</a>
      {query.data?.nextOffset != null && <a href={outlookHref(profile)}>Show all linked messages</a>}
    </div>
  </details>;
}
