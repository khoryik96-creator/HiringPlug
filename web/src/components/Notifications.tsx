// The bell in the top bar: new applicants on your jobs, your tasks due,
// offers waiting for your approval and your interviews today and tomorrow,
// with how many are new since you last opened it. And their settings in
// Settings › Your account: which kinds you get, and the morning email.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { SaveError } from "./SaveError";

type Kind = Schemas["NotificationKind"];
type List = Schemas["NotificationList"];

export const notificationsKey = ["notifications"] as const;

/** Each kind as Settings and the bell show it, and who can get it. */
export const KINDS: { kind: Kind; label: string; short: string; permission: Permission }[] = [
  { kind: "applicant", label: "New applicants on jobs you own", short: "Applicant", permission: "pipeline:read" },
  { kind: "task", label: "Your tasks due today or overdue", short: "Task", permission: "automations.tasks:read" },
  { kind: "offer_approval", label: "Offers waiting for your approval", short: "Offer", permission: "offers:approve" },
  { kind: "interview", label: "Your interviews today and tomorrow", short: "Interview", permission: "interviews:read" },
];
const SHORT = Object.fromEntries(KINDS.map((k) => [k.kind, k.short])) as Record<Kind, string>;

function useNotifications() {
  return useQuery({
    queryKey: notificationsKey,
    queryFn: async () => unwrap(await api.GET("/v1/notifications")),
    refetchInterval: 60_000,
    retry: false,
  });
}

function BellIcon() {
  return (
    <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" /><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
    </svg>
  );
}

export function NotificationBell() {
  const queryClient = useQueryClient();
  const list = useNotifications();
  const menu = useRef<HTMLDetailsElement>(null);
  // What was new when it was opened stays marked while it is open.
  const [shown, setShown] = useState<List | null>(null);
  const seen = useMutation({
    mutationFn: async () => { unwrap(await api.POST("/v1/notifications/seen")); },
    onSuccess: () => {
      queryClient.setQueryData<List>(notificationsKey, (old) => old && { ...old, unseen: 0, items: old.items.map((i) => ({ ...i, new: false })) });
    },
  });
  // A click outside closes it.
  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (menu.current?.open && !menu.current.contains(e.target as Node)) menu.current.open = false;
    };
    document.addEventListener("click", close);
    return () => { document.removeEventListener("click", close); };
  }, []);
  const data = list.data;
  if (!data) return null;
  const unseen = data.unseen;
  const view = shown ?? data;
  const label = unseen > 0 ? `Notifications: ${String(unseen)} new` : "Notifications";
  return (
    <details ref={menu} className="bell" onToggle={(e) => {
      if (e.currentTarget.open) {
        setShown(data);
        if (unseen > 0) seen.mutate();
      } else {
        setShown(null);
        void queryClient.invalidateQueries({ queryKey: notificationsKey });
      }
    }}>
      <summary className={`settings-link topbar-shortcut bell-button${unseen > 0 ? " has-new" : ""}`} aria-label={label} title={label}>
        <BellIcon />
        {unseen > 0 && <span className="bell-count" aria-hidden="true">{unseen > 9 ? "9+" : unseen}</span>}
      </summary>
      <div className="bell-menu" role="dialog" aria-label="Notifications">
        <div className="bell-head">
          <strong>Notifications</strong>
          <a href="#/account" onClick={() => { if (menu.current) menu.current.open = false; }}>Choose what you get</a>
        </div>
        {view.items.length === 0 ? (
          <p className="muted">Nothing needs you right now.</p>
        ) : (
          <ul>
            {view.items.map((it) => (
              <li key={it.id} className={`bell-item kind-${it.kind}${it.new ? " new" : ""}`}>
                <a href={it.href} onClick={() => { if (menu.current) menu.current.open = false; }}>
                  <span className="bell-kind">{SHORT[it.kind]}</span>
                  <span className="bell-title">{it.title}</span>
                  <span className="bell-detail muted">{it.detail}</span>
                </a>
              </li>
            ))}
          </ul>
        )}
      </div>
    </details>
  );
}

/** Settings › Your account: which notifications you get, and the morning email. */
export function NotificationSettings({ permissions }: { permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const list = useNotifications();
  const [saved, setSaved] = useState(false);
  const save = useMutation({
    mutationFn: async (body: Schemas["NotificationSettings"]) => unwrap(await api.PUT("/v1/notifications/settings", { body })),
    onSuccess: (s) => {
      queryClient.setQueryData<List>(notificationsKey, (old) => old && { ...old, ...s });
      void queryClient.invalidateQueries({ queryKey: notificationsKey });
      setSaved(true);
    },
  });
  const data = list.data;
  if (!data) return null;
  const kinds = KINDS.filter((k) => permissions.includes(k.permission));
  const put = (change: Partial<Schemas["NotificationSettings"]>) => {
    setSaved(false);
    save.mutate({ morningEmail: data.morningEmail, mutedKinds: data.mutedKinds, ...change });
  };
  return (
    <section className="panel notification-settings" aria-label="Notifications">
      <h3>Notifications</h3>
      <p className="muted">What shows in the bell at the top of every page, and in the morning email.</p>
      {kinds.map((k) => {
        const on = !data.mutedKinds.includes(k.kind);
        return (
          <label key={k.kind} className="switch-row">
            <input type="checkbox" role="switch" checked={on} disabled={save.isPending}
              onChange={() => { put({ mutedKinds: on ? [...data.mutedKinds, k.kind] : data.mutedKinds.filter((m) => m !== k.kind) }); }} />
            {k.label}
          </label>
        );
      })}
      <label className="switch-row morning">
        <input type="checkbox" role="switch" checked={data.morningEmail} disabled={save.isPending}
          onChange={() => { put({ morningEmail: !data.morningEmail }); }} />
        <span>Email me this list at 8:00 each morning <span className="muted">(only on mornings when there is something)</span></span>
      </label>
      {saved && <p role="status" className="muted">Saved.</p>}
      <SaveError error={save.error} shown={[]} onReload={() => { void list.refetch(); }} />
    </section>
  );
}
