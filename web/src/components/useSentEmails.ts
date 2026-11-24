// The emails sent about a candidate or to a contact, newest first. Reading
// the list again counts each email that shows a salary as read again (the
// log of who was shown a salary), so it is read when the page opens or after
// something changed it, never just because the window got focus. While some
// are on their way it asks only how they are getting on (statusOnly: no
// texts, not counted) and puts that in.
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";

type Email = Schemas["Email"];
type About = { candidateId: string } | { contactId: string };

/** How an email is getting on: what a status-only answer brings. */
const STATUS = ["status", "error", "sentAt", "deliveryMode", "bouncedTo", "bouncedAt"] as const;

const onItsWay = (emails: readonly Email[] | undefined) => emails?.some((e) => e.status === "queued") ?? false;

/** The emails with how each is getting on now, from a status-only answer. */
export function withStatus(emails: Email[] | undefined, latest: Email[] | undefined): Email[] | undefined {
  if (!emails || !latest) return emails;
  const byId = new Map(latest.map((e) => [e.id, e]));
  return emails.map((e) => {
    const now = byId.get(e.id);
    if (!now) return e;
    const status: Partial<Email> = {};
    for (const k of STATUS) Object.assign(status, { [k]: now[k] });
    return { ...e, ...status };
  });
}

export function useSentEmails(key: readonly unknown[], about: About) {
  const list = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/emails", { params: { query: about } })).items,
    refetchOnWindowFocus: (q) => !q.state.data?.some((e) => e.salary),
  });
  const status = useQuery({
    queryKey: [...key, "status"],
    queryFn: async () => unwrap(await api.GET("/v1/emails", { params: { query: { ...about, statusOnly: true } } })).items,
    enabled: onItsWay(list.data),
    refetchOnWindowFocus: false,
    // Sending happens in the background: look again while any is on its way.
    refetchInterval: (q) => (onItsWay(withStatus(list.data, q.state.data)) ? 3000 : false),
  });
  return { data: withStatus(list.data, status.data), error: list.error };
}
