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
import { JobDetail } from "./pages/JobDetail";
import { CvTemplates } from "./pages/CvTemplates";
import { Integrations } from "./pages/Integrations";
import { Privacy } from "./pages/Privacy";
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
  const section = /^#\/(jobs|companies|contacts|candidates|placements|reports|leads|import|integrations|privacy|team|account)/.exec(hash)?.[1] ?? "dashboard";
  return (
    <>
      <header className="row spread">
        <nav className="row" aria-label="Main">
          <strong>HiringPlug</strong>
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
          {permissions.includes("privacy:manage") && (
            <a href="#/privacy" aria-current={section === "privacy" ? "page" : undefined}>Data protection</a>
          )}
          {permissions.includes("identity.team:manage") && (
            <a href="#/team" aria-current={section === "team" ? "page" : undefined}>Team</a>
          )}
          {permissions.includes("integrations:manage") && (
            <a href="#/integrations" aria-current={section === "integrations" ? "page" : undefined}>Integrations</a>
          )}
        </nav>
        <span className="muted">
          <a href="#/account" aria-current={section === "account" ? "page" : undefined}>{me.data.user.name}</a> · {me.data.role.replace("_", " ")} · {me.data.tenant.name}{" "}
          <button type="button" className="link" onClick={onSignOut}>
            Sign out
          </button>
        </span>
      </header>
      <Page hash={hash} permissions={permissions} me={me.data} />
    </>
  );
}

/**
 * The page for a location. Detail pages get key={id}: a different record gets
 * a fresh page, so no edit state carries over.
 */
function Page({ hash, permissions, me }: { hash: string; permissions: Permission[]; me: Me }) {
  if (hash.startsWith("#/team") && permissions.includes("identity.team:manage")) return <Team myRole={me.role} myId={me.user.id} />;
  if (hash.startsWith("#/account")) return <><h2>Your account</h2><ChangePassword /></>;
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
  if (hash.startsWith("#/cv-templates") && permissions.includes("cvformats:create")) {
    return <CvTemplates canManage={permissions.includes("cvformats.templates:manage")} />;
  }
  if (hash.startsWith("#/email-templates") && permissions.includes("emails:send")) {
    return <EmailTemplates canManage={permissions.includes("emails.templates:manage")} />;
  }
  if (hash.startsWith("#/placements") && permissions.includes("placements:read")) return <Placements permissions={permissions} />;
  if (hash.startsWith("#/leads") && permissions.includes("leads:read")) return <Leads permissions={permissions} />;
  if (hash.startsWith("#/reports") && permissions.includes("reports:read")) return <Reports />;
  if (hash.startsWith("#/integrations") && permissions.includes("integrations:manage")) return <Integrations />;
  if (hash.startsWith("#/privacy") && permissions.includes("privacy:manage")) return <Privacy />;
  if (hash.startsWith("#/import")) return <ImportPage permissions={permissions} />;
  if (hash.startsWith("#/candidates")) return <CandidateList canCreate={permissions.includes("candidates:create")} />;
  if (permissions.includes("pipeline:read")) return <Dashboard permissions={permissions} />;
  return <CandidateList canCreate={permissions.includes("candidates:create")} />;
}

export function App() {
  useHash(); // re-render on navigation
  const [signedIn, setSignedIn] = useState(getToken() !== null);
  // The public careers page: never behind sign-in. (hash re-renders on change.)
  const careers = careersRoute(window.location);
  // Invitation and password reset links: also never behind sign-in.
  const link = authLinkRoute(window.location.hash);
  const signOut = () => {
    clearToken();
    queryClient.clear();
    setSignedIn(false);
  };
  return (
    <QueryClientProvider client={queryClient}>
      {careers ? (
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
