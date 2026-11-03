import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { setToken } from "../auth";
import { ErrorMessage } from "../components/ErrorMessage";
import { SaveError } from "../components/SaveError";

/** Choose a password with an invitation or password reset link, then sign in. */
export function ChoosePassword({ token, onSignedIn }: { token: string; onSignedIn: () => void }) {
  const link = useQuery({
    queryKey: ["auth-link", token],
    queryFn: async () => unwrap(await api.GET("/v1/auth/links/{linkToken}", { params: { path: { linkToken: token } } })),
  });
  const [form, setForm] = useState({ name: "", password: "", repeat: "" });
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/auth/links/{linkToken}", {
      params: { path: { linkToken: token } },
      body: { password: form.password, ...(link.data?.kind === "invite" && form.name.trim() ? { name: form.name.trim() } : {}) },
    })),
    onSuccess: (s) => {
      setToken(s.token);
      window.location.hash = "#/";
      onSignedIn();
    },
  });
  const l = link.data;
  return (
    <main className="narrow">
      <h1>HiringPlug</h1>
      <ErrorMessage error={link.error} />
      {link.error && <a href="#/">Go to sign in</a>}
      {l && (
        <form className="form-grid" aria-label="Choose your password" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
          <p className="full">
            {l.kind === "invite" ? <>Welcome to <strong>{l.companyName}</strong>. Choose a password to join.</> : <>Choose a new password for {l.companyName}.</>}
          </p>
          <p className="full muted">{l.email}</p>
          {l.kind === "invite" && (
            <label className="field full">Your name<input value={form.name} placeholder={l.name} onChange={(e) => { setForm({ ...form, name: e.target.value }); }} /></label>
          )}
          <input type="email" autoComplete="username" value={l.email} readOnly hidden />
          <label className="field full">Password (at least 10 characters)
            <input type="password" autoComplete="new-password" value={form.password} onChange={(e) => { setForm({ ...form, password: e.target.value }); }} />
          </label>
          <label className="field full">The same again
            <input type="password" autoComplete="new-password" value={form.repeat} onChange={(e) => { setForm({ ...form, repeat: e.target.value }); }} />
          </label>
          {form.repeat && form.repeat !== form.password && <p className="full warning" role="status">The two passwords differ.</p>}
          <SaveError error={save.error} shown={[]} onReload={() => undefined} />
          <div className="row full">
            <button type="submit" disabled={save.isPending || form.password.length < 10 || form.password !== form.repeat}>
              {l.kind === "invite" ? "Join" : "Save password"}
            </button>
          </div>
        </form>
      )}
    </main>
  );
}
