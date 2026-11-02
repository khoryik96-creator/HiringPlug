import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, ApiError, type Permission, unwrap } from "./api/client";
import { clearToken, getToken } from "./auth";
import { ErrorMessage } from "./components/ErrorMessage";
import { CandidateDetail } from "./pages/CandidateDetail";
import { CandidateList } from "./pages/CandidateList";
import { CompanyDetail } from "./pages/CompanyDetail";
import { CompanyList } from "./pages/CompanyList";
import { SignIn } from "./pages/SignIn";

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
  const section = hash.startsWith("#/companies") ? "companies" : "candidates";
  return (
    <>
      <header className="row spread">
        <nav className="row" aria-label="Main">
          <strong>HiringPlug</strong>
          <a href="#/candidates" aria-current={section === "candidates" ? "page" : undefined}>Candidates</a>
          <a href="#/companies" aria-current={section === "companies" ? "page" : undefined}>Companies</a>
        </nav>
        <span className="muted">
          {me.data.user.name} · {me.data.role.replace("_", " ")} · {me.data.tenant.name}{" "}
          <button type="button" className="link" onClick={onSignOut}>
            Sign out
          </button>
        </span>
      </header>
      <Page hash={hash} permissions={permissions} />
    </>
  );
}

/**
 * The page for a location. Detail pages get key={id}: a different record gets
 * a fresh page, so no edit state carries over.
 */
function Page({ hash, permissions }: { hash: string; permissions: Permission[] }) {
  const candidate = /^#\/candidates\/(cand_[0-9a-z]+)$/.exec(hash)?.[1];
  if (candidate) return <CandidateDetail key={candidate} id={candidate} permissions={permissions} />;
  const company = /^#\/companies\/(comp_[0-9a-z]+)$/.exec(hash)?.[1];
  if (company) return <CompanyDetail key={company} id={company} permissions={permissions} />;
  if (hash.startsWith("#/companies")) return <CompanyList canCreate={permissions.includes("companies:create")} />;
  return <CandidateList canCreate={permissions.includes("candidates:create")} />;
}

export function App() {
  const [signedIn, setSignedIn] = useState(getToken() !== null);
  const signOut = () => {
    clearToken();
    queryClient.clear();
    setSignedIn(false);
  };
  return (
    <QueryClientProvider client={queryClient}>
      {signedIn ? <Shell onSignOut={signOut} /> : <SignIn onSignedIn={() => { setSignedIn(true); }} />}
    </QueryClientProvider>
  );
}
