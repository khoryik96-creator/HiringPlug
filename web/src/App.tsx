import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, ApiError, type Me, type Permission, unwrap } from "./api/client";
import { authLinkRoute, clearToken, getToken } from "./auth";
import { ErrorMessage } from "./components/ErrorMessage";
import { CandidateDetail } from "./pages/CandidateDetail";
import { CandidateList } from "./pages/CandidateList";
import { careersRoute } from "./careersRoute";
import { Careers } from "./pages/Careers";
import { CompareCvs } from "./pages/CompareCvs";
import { CompanyDetail } from "./pages/CompanyDetail";
import { ContactDetail } from "./pages/ContactDetail";
import { ContactList } from "./pages/ContactList";
import { Dashboard } from "./pages/Dashboard";
import { EmailTemplates } from "./pages/EmailTemplates";
import { ImportPage } from "./pages/ImportPage";
import { ScreeningCall } from "./pages/ScreeningCall";
import { JobDetail } from "./pages/JobDetail";
import { CvTemplates } from "./pages/CvTemplates";
import { Integrations } from "./pages/Integrations";
import { Privacy } from "./pages/Privacy";
import { Access } from "./pages/Access";
import { AiProvider } from "./pages/AiProvider";
import { BookingPage, bookingRoute } from "./pages/BookingPage";
import { BookingHours } from "./components/BookingHours";
import { PageErrorBoundary } from "./components/PageErrorBoundary";
import { SETTINGS_PAGES, SettingsLayout, settingsPages } from "./components/SettingsLayout";
import { Settings } from "./pages/Settings";
import { JobList } from "./pages/JobList";
import { Leads } from "./pages/Leads";
import { Placements } from "./pages/Placements";
import { Reports } from "./pages/Reports";
import { CompanyList } from "./pages/CompanyList";
import { ChoosePassword } from "./pages/ChoosePassword";
import { SignIn } from "./pages/SignIn";
import { ChangePassword, Team } from "./pages/Team";

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
  const section = /^#\/(jobs|companies|contacts|candidates|placements|reports|leads|import|integrations|privacy|access|team|settings|ai-provider|account|cv-templates|email-templates)/.exec(hash)?.[1] ?? "dashboard";
  const inSettings = SETTINGS_PAGES.some(([key]) => key === section);
  return (
    <>
      <header className="topbar">
        <span className="brand">Hiring<span>Plug</span></span>
        <nav aria-label="Main">
          <a href="#/" aria-current={section === "dashboard" ? "page" : undefined}>Dashboard</a>
          <a href="#/jobs" aria-current={section === "jobs" ? "page" : undefined}>Jobs</a>
          <a href="#/candidates" aria-current={section === "candidates" ? "page" : undefined}>Candidates</a>
          <a href="#/companies" aria-current={section === "companies" ? "page" : undefined}>Companies</a>
          <a href="#/contacts" aria-current={section === "contacts" ? "page" : undefined}>Contacts</a>
          {permissions.includes("placements:read") && (
            <a href="#/placements" aria-current={section === "placements" ? "page" : undefined}>Placements</a>
          )}
          {permissions.includes("leads:read") && (
            <a href="#/leads" aria-current={section === "leads" ? "page" : undefined}>Leads</a>
          )}
          {permissions.includes("reports:read") && (
            <a href="#/reports" aria-current={section === "reports" ? "page" : undefined}>Reports</a>
          )}
          <a href="#/import" aria-current={section === "import" ? "page" : undefined}>Import</a>
        </nav>
        <a className="settings-link" href="#/settings" aria-current={inSettings ? "page" : undefined}>
          <svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
          </svg>
          Settings
        </a>
        <div className="who">
          <a href="#/account" aria-current={section === "account" ? "page" : undefined}>
            <span className="avatar" aria-hidden="true">{initials(me.data.user.name)}</span>
            <span className="name"><strong>{me.data.user.name}</strong><small>{me.data.role.replace("_", " ")} · {me.data.tenant.name}</small></span>
          </a>
          <button type="button" className="link" onClick={onSignOut}>
            Sign out
          </button>
        </div>
      </header>
      {/* key: a new location starts with a fresh boundary */}
      <PageErrorBoundary key={hash}>
        <Page hash={hash} permissions={permissions} me={me.data} />
      </PageErrorBoundary>
    </>
  );
}

/**
 * The page for a location. Detail pages get key={id}: a different record gets
 * a fresh page, so no edit state carries over.
 */
function Page({ hash, permissions, me }: { hash: string; permissions: Permission[]; me: Me }) {
  const setting = settingsSection(hash, permissions);
  if (setting) {
    return <SettingsLayout section={setting} permissions={permissions}><SettingsPage section={setting} permissions={permissions} me={me} /></SettingsLayout>;
  }
  const screening = /^#\/candidates\/(cand_[0-9a-z]+)\/screening$/.exec(hash)?.[1];
  if (screening) return <ScreeningCall key={screening} candidateId={screening} permissions={permissions} />;
  const compare = /^#\/candidates\/(cand_[0-9a-z]+)\/compare$/.exec(hash)?.[1];
  if (compare) return <CompareCvs key={compare} candidateId={compare} />;
  const candidate = /^#\/candidates\/(cand_[0-9a-z]+)$/.exec(hash)?.[1];
  if (candidate) return <CandidateDetail key={candidate} id={candidate} permissions={permissions} />;
  const company = /^#\/companies\/(comp_[0-9a-z]+)$/.exec(hash)?.[1];
  if (company) return <CompanyDetail key={company} id={company} permissions={permissions} />;
  const job = /^#\/jobs\/(jo_[0-9a-z]+)$/.exec(hash)?.[1];
  if (job) return <JobDetail key={job} id={job} permissions={permissions} />;
  if (hash.startsWith("#/jobs")) return <JobList canCreate={permissions.includes("jobs:create")} />;
  const contact = /^#\/contacts\/(cont_[0-9a-z]+)$/.exec(hash)?.[1];
  if (contact) return <ContactDetail key={contact} id={contact} permissions={permissions} />;
  if (hash.startsWith("#/contacts")) return <ContactList canCreate={permissions.includes("contacts:create")} />;
  if (hash.startsWith("#/companies")) return <CompanyList canCreate={permissions.includes("companies:create")} />;
  if (hash.startsWith("#/placements") && permissions.includes("placements:read")) return <Placements permissions={permissions} />;
  if (hash.startsWith("#/leads") && permissions.includes("leads:read")) return <Leads permissions={permissions} />;
  if (hash.startsWith("#/reports") && permissions.includes("reports:read")) return <Reports />;
  if (hash.startsWith("#/import")) return <ImportPage permissions={permissions} />;
  if (hash.startsWith("#/candidates")) return <CandidateList canCreate={permissions.includes("candidates:create")} />;
  if (permissions.includes("pipeline:read")) return <Dashboard permissions={permissions} />;
  return <CandidateList canCreate={permissions.includes("candidates:create")} />;
}

/**
 * The part of Settings a location opens, if the user may open it. Settings
 * itself opens its first part they may open (everyone has their account).
 */
function settingsSection(hash: string, permissions: Permission[]): string | null {
  const key = /^#\/([a-z-]+)/.exec(hash)?.[1];
  if (!SETTINGS_PAGES.some(([k]) => k === key)) return null;
  const allowed = settingsPages(permissions);
  if (allowed.some(([k]) => k === key)) return key ?? null;
  return key === "settings" ? allowed[0]?.[0] ?? null : null;
}

function SettingsPage({ section, permissions, me }: { section: string; permissions: Permission[]; me: Me }) {
  switch (section) {
    case "settings": return <Settings />;
    case "ai-provider": return <AiProvider />;
    case "team": return <Team myRole={me.role} myId={me.user.id} />;
    case "access": return <Access />;
    case "privacy": return <Privacy />;
    case "integrations": return <Integrations />;
    case "cv-templates": return <CvTemplates canManage={permissions.includes("cvformats.templates:manage")} />;
    case "email-templates": return <EmailTemplates canManage={permissions.includes("emails.templates:manage")} />;
    default: return <main><h2>Your account</h2>{permissions.includes("scheduling:use") && <BookingHours />}<ChangePassword /></main>;
  }
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
