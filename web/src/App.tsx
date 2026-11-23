import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { Suspense, useEffect, useRef, useState, useSyncExternalStore, type KeyboardEvent } from "react";

import { api, ApiError, unwrap } from "./api/client";
import { authLinkRoute, clearToken, getToken } from "./auth";
import { AiNotice } from "./components/AiNotice";
import { CloseIcon, DockRightIcon, IconButton, OutlookIcon } from "./components/ActionIcons";
import { ErrorMessage } from "./components/ErrorMessage";
import { CvImportIndicator } from "./components/outlook/CvImportIndicator";
import { NotificationBell } from "./components/Notifications";
import { careersRoute } from "./careersRoute";
import { PageErrorBoundary } from "./components/PageErrorBoundary";
import { OpenTabs } from "./components/OpenTabs";
import { SettingsLayout } from "./components/SettingsLayout";
import { BookingPage, bookingRoute } from "./pages/BookingPage";
import { ClientShortlistPage, shortlistRoute } from "./pages/ClientShortlistPage";
import { Careers } from "./pages/Careers";
import { ChoosePassword } from "./pages/ChoosePassword";
import { SignIn } from "./pages/SignIn";
import { Menu } from "./components/Menu";
import { Calendar, menu, resolve, type Route } from "./routes";

const newSessionQueries = () => new QueryClient({ defaultOptions: { queries: { retry: false } } });

function useHash(): string {
  const [hash, setHash] = useState(window.location.hash);
  useEffect(() => {
    const onChange = () => { setHash(window.location.hash); };
    window.addEventListener("hashchange", onChange);
    return () => { window.removeEventListener("hashchange", onChange); };
  }, []);
  return hash;
}

/** Ends this session on the server too, so a copied token stops working; signing out locally never waits on it failing. */
async function endSession(): Promise<void> {
  try {
    await api.POST("/v1/auth/sign-out");
  } catch {
    // Offline or already signed out: the token still expires on its own.
  }
}

const calendarOverlayWidth = "(max-width: 1050px)";
function watchCalendarWidth(change: () => void) {
  if (typeof window.matchMedia !== "function") return () => {};
  const media = window.matchMedia(calendarOverlayWidth);
  media.addEventListener("change", change);
  return () => { media.removeEventListener("change", change); };
}
const calendarOverlaysPage = () => typeof window.matchMedia === "function" && window.matchMedia(calendarOverlayWidth).matches;

function Shell({ onSignOut }: { onSignOut: () => void }) {
  const hash = useHash();
  const [calendarDocked, setCalendarDocked] = useState(false);
  // On a phone the pages are under a Menu button; opened at a page, it closes on going to another.
  const [menuOpenAt, setMenuOpenAt] = useState<string | null>(null);
  const calendarOverlay = useSyncExternalStore(watchCalendarWidth, calendarOverlaysPage);
  const calendarDockButton = useRef<HTMLButtonElement>(null);
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  if (me.error instanceof ApiError && me.error.code === "UNAUTHENTICATED") {
    onSignOut();
    return null;
  }
  // Only without data: a failed background refresh must not unmount the page
  // (and any form being filled in) for the last loaded profile.
  if (!me.data) return me.error ? <ErrorMessage error={me.error} /> : <p className="muted">Loading…</p>;

  const permissions = me.data.permissions;
  const { route, id } = resolve(hash, permissions);
  const settingsPages = menu("settings", permissions);
  const inSettings = route.menu?.in === "settings";
  // Links to editors within a settings page should keep its forms mounted.
  const pageKey = inSettings ? hash.split("#", 2).join("#") : hash;
  const canUseCalendar = permissions.includes("calendars:use");
  const onCalendar = route.section === "calendar";
  const showCalendarDock = canUseCalendar && calendarDocked && !onCalendar;
  const calendarModal = showCalendarDock && calendarOverlay;
  const closeCalendarDock = () => {
    setCalendarDocked(false);
    if (calendarModal) {
      requestAnimationFrame(() => { calendarDockButton.current?.focus(); });
    } else calendarDockButton.current?.focus();
  };
  const menuOpen = menuOpenAt === hash;
  return (
    <div className={showCalendarDock ? "app-shell calendar-docked" : "app-shell"}>
      <header className={menuOpen ? "topbar menu-open" : "topbar"} inert={calendarModal}>
        <span className="brand">Hiring<span>Plug</span></span>
        <span className="app-version" title="HiringPlug version">v{__APP_VERSION__}</span>
        <MainMenu routes={menu("main", permissions)} current={route.section} />
        <div className="topbar-tools">
          {permissions.includes("emails:send") && <a className="settings-link topbar-shortcut" href="#/outlook" aria-label="Outlook" title="Outlook"
            aria-current={route.section === "outlook" ? "page" : undefined}><OutlookIcon /></a>}
          {canUseCalendar && <a className="settings-link topbar-shortcut" href="#/calendar" aria-label="Calendar" title="Calendar"
            aria-current={onCalendar ? "page" : undefined} onClick={() => { setCalendarDocked(false); }}>
            <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="5" width="18" height="16" rx="2" />
              <path d="M8 3v4m8-4v4M3 10h18m-13 5h2m4 0h2" />
            </svg>
          </a>}
          {canUseCalendar && !onCalendar && <button type="button" ref={calendarDockButton} className="settings-link topbar-shortcut calendar-dock-toggle"
            aria-label="Dock calendar" title={showCalendarDock ? "Close calendar dock" : "Dock calendar"} aria-pressed={showCalendarDock}
            aria-controls={showCalendarDock ? "calendar-dock" : undefined} onClick={() => { setCalendarDocked(!showCalendarDock); }}><DockRightIcon /></button>}
          {permissions.includes("intake:create") && <CvImportIndicator userId={me.data.user.id} />}
          {permissions.includes("notifications:read") && <NotificationBell />}
          <a className="settings-link topbar-shortcut" href="#/help" aria-label="Help" title="Help"
            aria-current={route.section === "help" ? "page" : undefined}>
            <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="9.5" />
              <path d="M9.2 9.3a2.9 2.9 0 0 1 5.6 1c0 1.9-2.8 2.5-2.8 4.2" />
              <path d="M12 17.6h.01" />
            </svg>
          </a>
          <a className="settings-link" href="#/settings" title="Settings" aria-current={inSettings ? "page" : undefined}>
            <svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
            </svg>
            <span className="settings-label">Settings</span>
          </a>
          <button type="button" className="settings-link topbar-shortcut topbar-menu-button" aria-expanded={menuOpen} aria-controls="topbar-menu"
            aria-label={menuOpen ? "Close menu" : "Menu"} onClick={() => { setMenuOpenAt(menuOpen ? null : hash); }}>
            {menuOpen ? <CloseIcon /> : <svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M4 7h16M4 12h16M4 17h16" /></svg>}
          </button>
        </div>
        <div className="who">
          <a href="#/account" aria-label={`Your account: ${me.data.user.name}`} aria-current={route.section === "account" ? "page" : undefined}>
            <span className="avatar" aria-hidden="true">{initials(me.data.user.name)}</span>
            <span className="name"><strong>{me.data.user.name}</strong><small>{me.data.role.replace("_", " ")} · {me.data.tenant.name}</small></span>
          </a>
          <button type="button" className="link" onClick={() => { void endSession().finally(onSignOut); }}>
            Sign out
          </button>
        </div>
      </header>
      <div className="shell-content">
        <div className="shell-page" inert={calendarModal}>
          <AiNotice permissions={permissions} />
          <OpenTabs hash={hash} />
          <PageErrorBoundary key={pageKey}>
            <Suspense fallback={<p className="muted page-loading">Loading…</p>}>
              {route.menu?.in === "settings" ? (
                <SettingsLayout section={route.section} pages={settingsPages} permissions={permissions}>
                  {route.render({ permissions, me: me.data, id })}
                </SettingsLayout>
              ) : route.render({ permissions, me: me.data, id })}
            </Suspense>
          </PageErrorBoundary>
        </div>
        {calendarModal && <div className="calendar-dock-backdrop" aria-hidden="true" />}
        {showCalendarDock && <CalendarDock userId={me.data.user.id} modal={calendarModal} onClose={closeCalendarDock} />}
      </div>
    </div>
  );
}

function CalendarDock({ userId, modal, onClose }: { userId: string; modal: boolean; onClose: () => void }) {
  const dock = useRef<HTMLElement>(null);
  useEffect(() => { dock.current?.focus(); }, []);
  const onKey = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === "Escape" && !event.defaultPrevented) onClose();
    // An appointment is a separate dialog rendered in a portal.
    if (!modal || event.key !== "Tab" || !event.currentTarget.contains(event.target as Node)) return;
    const controls = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("a[href], button, input, select, textarea, summary, [tabindex]"))
      .filter((element) => element.tabIndex >= 0 && !element.matches(":disabled") && element.getClientRects().length > 0);
    const boundary = event.shiftKey ? controls[0] : controls.at(-1);
    if (event.target === boundary || event.target === event.currentTarget) {
      event.preventDefault();
      (event.shiftKey ? controls.at(-1) : controls[0])?.focus();
    }
  };
  return (
    <aside ref={dock} id="calendar-dock" className="calendar-dock" aria-label="Calendar dock" tabIndex={-1}
      role={modal ? "dialog" : undefined} aria-modal={modal || undefined} onKeyDown={onKey}>
      <header className="calendar-dock-head">
        <h2>Calendar</h2>
        <IconButton label="Close calendar dock" icon={<CloseIcon />} onClick={onClose} />
      </header>
      <PageErrorBoundary>
        <Suspense fallback={<p className="muted" role="status">Loading calendar…</p>}><Calendar docked userId={userId} /></Suspense>
      </PageErrorBoundary>
    </aside>
  );
}

export function App() {
  useHash(); // re-render on navigation
  const [signedIn, setSignedIn] = useState(getToken() !== null);
  const [queryClient, setQueryClient] = useState(newSessionQueries);
  // The public careers page: never behind sign-in. (hash re-renders on change.)
  const careers = careersRoute(window.location);
  // A booking page: also public.
  const booking = bookingRoute(window.location);
  // A client's shortlist page: also public.
  const shortlist = shortlistRoute(window.location);
  // Invitation and password reset links: also never behind sign-in.
  const link = authLinkRoute(window.location.hash);
  const changeSession = (signedIn: boolean) => {
    // Pending mutations retain their original client. Give the next sign-in
    // a separate client so late responses cannot populate its cache.
    queryClient.clear();
    setQueryClient(newSessionQueries());
    setSignedIn(signedIn);
  };
  const signOut = () => {
    clearToken();
    changeSession(false);
  };
  return (
    <QueryClientProvider client={queryClient}>
      {booking ? (
        <BookingPage key={booking} token={booking} />
      ) : shortlist ? (
        <ClientShortlistPage key={shortlist} token={shortlist} />
      ) : careers ? (
        // Public: candidates are never asked to sign in.
        <Careers key={careers.slug} route={careers} pathStyle={!window.location.hash.startsWith("#/careers/")} />
      ) : link ? (
        <ChoosePassword key={link.token} token={link.token} onSignedIn={() => { changeSession(true); }} />
      ) : signedIn ? (
        <Shell onSignOut={signOut} />
      ) : (
        <SignIn onSignedIn={() => { changeSession(true); }} />
      )}
    </QueryClientProvider>
  );
}

/** "Demo Recruiter" → "DR": the letters on the account button. */
export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  return words.slice(0, 2).map((w) => w[0]?.toUpperCase() ?? "").join("") || "?";
}

/** The top bar's pages; the less used ones (Leads, Reports, Import) under "More". */
function MainMenu({ routes, current }: { routes: Route[]; current: string }) {
  const link = (r: Route, close?: () => void) => (
    <a key={r.section} href={r.menu?.href} aria-current={current === r.section ? "page" : undefined} onClick={close}>{r.menu?.label}</a>
  );
  const more = routes.filter((r) => r.menu?.more);
  const open = more.find((r) => r.section === current);
  return (
    <nav id="topbar-menu" aria-label="Main">
      {routes.filter((r) => !r.menu?.more).map((r) => link(r))}
      {more.length > 0 && (
        <Menu label={<>{open?.menu?.label ?? "More"} <span aria-hidden="true">▾</span></>} ariaLabel="More pages"
          className={open ? "more-pages here" : "more-pages"}>
          {(close) => more.map((r) => link(r, close))}
        </Menu>
      )}
    </nav>
  );
}
