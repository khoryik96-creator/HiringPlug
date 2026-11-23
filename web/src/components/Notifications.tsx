// The bell in the top bar: new applicants on your jobs, your tasks due,
// offers waiting for your approval, your interviews today and tomorrow and
// the CVs you had formatted (green the moment one is ready), with how many
// are new since you last opened it. Open, it is marked as the one clicked,
// and it closes by itself after 30 seconds. And their settings in
// Settings › Your account: which kinds you get, and the morning email.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { SaveError } from "./SaveError";
import { useTopbarPanel } from "./topbarPanel";

type Kind = Schemas["NotificationKind"];
type List = Schemas["NotificationList"];

export const notificationsKey = ["notifications"] as const;

/** Each kind as Settings and the bell show it, and who can get it. */
export const KINDS: { kind: Kind; label: string; short: string; permission: Permission }[] = [
  { kind: "applicant", label: "New applicants on jobs you own", short: "Applicant", permission: "pipeline:read" },
  { kind: "task", label: "Your tasks due today or overdue", short: "Task", permission: "automations.tasks:read" },
  { kind: "offer_approval", label: "Offers waiting for your approval", short: "Offer", permission: "offers:approve" },
  { kind: "interview", label: "Your interviews today and tomorrow", short: "Interview", permission: "interviews:read" },
  { kind: "cv_formatted", label: "CVs you had formatted, ready or failed", short: "CV ready", permission: "cvformats:create" },
];
const SHORT = Object.fromEntries(KINDS.map((k) => [k.kind, k.short])) as Record<Kind, string>;

/** The bell's list (shared: the open-record tabs read it too, with no request of their own). */
export function useNotifications() {
  return useQuery({
    queryKey: notificationsKey,
    queryFn: async () => unwrap(await api.GET("/v1/notifications")),
    refetchInterval: 30_000, // a formatted CV is said soon after it is ready
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
    // Up to the list shown (asOf): what came after it stays new. A refresh
    // already on its way would bring the old count back: it is cancelled.
    mutationFn: async (upTo: string) => {
      await queryClient.cancelQueries({ queryKey: notificationsKey });
      unwrap(await api.POST("/v1/notifications/seen", { body: { upTo } }));
    },
    onSuccess: () => {
      queryClient.setQueryData<List>(notificationsKey, (old) => old && { ...old, unseen: 0, items: old.items.map((i) => ({ ...i, new: false })) });
    },
  });
  // Marked while open; a click elsewhere or 30 seconds closes it.
  const { open, track } = useTopbarPanel(menu);
  const data = list.data;
  if (!data) return null;
  const unseen = data.unseen;
  const view = shown ?? data;
  // A CV formatted since last looked lights the bell green.
  const cvReady = data.items.some((i) => i.kind === "cv_formatted" && i.new);
  const label = unseen > 0 ? `Notifications: ${String(unseen)} new${cvReady ? ", a CV is formatted" : ""}` : "Notifications";
  return (
    <details ref={menu} className="bell" onToggle={(e) => {
      track(e);
      if (e.currentTarget.open) {
        setShown(data);
        if (unseen > 0) seen.mutate(data.asOf);
      } else {
        setShown(null);
        void queryClient.invalidateQueries({ queryKey: notificationsKey });
      }
    }}>
      <summary className={`settings-link topbar-shortcut bell-button${unseen > 0 ? " has-new" : ""}${cvReady ? " cv-ready" : ""}`}
        aria-label={label} title={label} aria-expanded={open}>
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
  if (!data) {
    return list.isError ? (
      <section className="panel notification-settings" aria-label="Notifications">
        <h3>Notifications</h3>
        <ErrorMessage error={list.error} />
        <button type="button" className="link" onClick={() => { void list.refetch(); }}>Try again</button>
      </section>
    ) : null;
  }
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
