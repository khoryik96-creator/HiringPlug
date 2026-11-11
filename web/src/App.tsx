import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { Suspense, useEffect, useState } from "react";

import { api, ApiError, unwrap } from "./api/client";
import { authLinkRoute, clearToken, getToken } from "./auth";
import { ErrorMessage } from "./components/ErrorMessage";
import { careersRoute } from "./careersRoute";
import { PageErrorBoundary } from "./components/PageErrorBoundary";
import { SettingsLayout } from "./components/SettingsLayout";
import { BookingPage, bookingRoute } from "./pages/BookingPage";
import { Careers } from "./pages/Careers";
import { ChoosePassword } from "./pages/ChoosePassword";
import { SignIn } from "./pages/SignIn";
import { menu, resolve } from "./routes";

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

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

function Shell({ onSignOut }: { onSignOut: () => void }) {
  const hash = useHash();
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
  const inSettings = route.menu?.in === "settings" || menu("settings", permissions).some((r) => r.section === route.section);
  return (
    <>
      <header className="topbar">
        <span className="brand">Hiring<span>Plug</span></span>
        <nav aria-label="Main">
          {menu("main", permissions).map((r) => (
            <a key={r.section} href={r.menu?.href} aria-current={route.section === r.section ? "page" : undefined}>{r.menu?.label}</a>
          ))}
        </nav>
        <a className="settings-link" href="#/settings" aria-current={inSettings ? "page" : undefined}>
          <svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
          </svg>
          Settings
        </a>
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
      {/* key: a new location starts with a fresh boundary */}
      <PageErrorBoundary key={hash}>
        <Suspense fallback={<p className="muted page-loading">Loading…</p>}>
          {route.menu?.in === "settings" ? (
            <SettingsLayout section={route.section} pages={menu("settings", permissions)}>
              {route.render({ permissions, me: me.data, id })}
            </SettingsLayout>
          ) : route.render({ permissions, me: me.data, id })}
        </Suspense>
      </PageErrorBoundary>
    </>
  );
}

export function App() {
  useHash(); // re-render on navigation
  const [signedIn, setSignedIn] = useState(getToken() !== null);
  // The public careers page: never behind sign-in. (hash re-renders on change.)
  const careers = careersRoute(window.location);
  // A booking page: also public.
  const booking = bookingRoute(window.location);
  // Invitation and password reset links: also never behind sign-in.
  const link = authLinkRoute(window.location.hash);
  const signOut = () => {
    clearToken();
    queryClient.clear();
    setSignedIn(false);
  };
  return (
    <QueryClientProvider client={queryClient}>
      {booking ? (
        <BookingPage key={booking} token={booking} />
      ) : careers ? (
        // Public: candidates are never asked to sign in.
        <Careers key={careers.slug} route={careers} pathStyle={!window.location.hash.startsWith("#/careers/")} />
      ) : link ? (
        <ChoosePassword key={link.token} token={link.token} onSignedIn={() => { queryClient.clear(); setSignedIn(true); }} />
      ) : signedIn ? (
        <Shell onSignOut={signOut} />
      ) : (
        <SignIn onSignedIn={() => { setSignedIn(true); }} />
      )}
    </QueryClientProvider>
  );
}

/** "Demo Recruiter" → "DR": the letters on the account button. */
export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  return words.slice(0, 2).map((w) => w[0]?.toUpperCase() ?? "").join("") || "?";
}
