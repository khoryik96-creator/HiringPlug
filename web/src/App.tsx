import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, ApiError, unwrap } from "./api/client";
import { clearToken, getToken } from "./auth";
import { ErrorMessage } from "./components/ErrorMessage";
import { CandidateDetail } from "./pages/CandidateDetail";
import { CandidateList } from "./pages/CandidateList";
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
  if (me.error) return <ErrorMessage error={me.error} />;
  if (!me.data) return <p className="muted">Loading…</p>;

  const detail = /^#\/candidates\/(cand_[0-9a-z]+)$/.exec(hash);
  return (
    <>
      <header className="row spread">
        <strong>HiringPlug</strong>
        <span className="muted">
          {me.data.user.name} · {me.data.role.replace("_", " ")} · {me.data.tenant.name}{" "}
          <button type="button" className="link" onClick={onSignOut}>
            Sign out
          </button>
        </span>
      </header>
      {detail?.[1] ? (
        <CandidateDetail id={detail[1]} permissions={me.data.permissions} />
      ) : (
        <CandidateList canCreate={me.data.permissions.includes("candidates:create")} />
      )}
    </>
  );
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
