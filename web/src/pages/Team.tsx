import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { setToken } from "../auth";
import { ErrorMessage } from "../components/ErrorMessage";
import { SaveError } from "../components/SaveError";
import { formatWhen, keysOf, ROLE, TEAM_STATUS } from "../labels";

type Member = Schemas["TeamMember"];
const teamKey = ["team"] as const;

/** Who is in the company: invite people, change roles, deactivate (owners and admins). */
export function Team({ myRole, myId }: { myRole: Schemas["Role"]; myId: string }) {
  const list = useQuery({ queryKey: teamKey, queryFn: async () => unwrap(await api.GET("/v1/team")).items });
  return (
    <>
      <h2>Team</h2>
      <ErrorMessage error={list.error} />
      {list.data && (
        <table>
          <thead><tr><th>Name</th><th>Email</th><th>Role</th><th>Status</th><th>Last signed in</th><th /></tr></thead>
          <tbody>{list.data.map((m) => <MemberRow key={m.id} member={m} myRole={myRole} me={m.id === myId} />)}</tbody>
        </table>
      )}
      <Invite myRole={myRole} />
    </>
  );
}

const roles = (myRole: Schemas["Role"]) => keysOf(ROLE).filter((r) => r !== "owner" || myRole === "owner");

function MemberRow({ member: m, myRole, me }: { member: Member; myRole: Schemas["Role"]; me: boolean }) {
  const queryClient = useQueryClient();
  const [link, setLink] = useState<string | null>(null);
  const update = useMutation({
    mutationFn: async (body: Schemas["TeamMemberUpdate"]) =>
      unwrap(await api.PATCH("/v1/team/{userId}", { params: { path: { userId: m.id } }, body })),
    onSettled: async () => { await queryClient.invalidateQueries({ queryKey: teamKey }); },
  });
  const resend = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/team/{userId}/invite", { params: { path: { userId: m.id } } })),
    onSuccess: (inv) => { setLink(inv.link); },
  });
  const locked = me || (m.role === "owner" && myRole !== "owner");
  return (
    <tr>
      <td>{m.name}{me && <span className="muted"> (you)</span>}</td>
      <td>{m.email}</td>
      <td>
        {locked ? ROLE[m.role] : (
          <select aria-label={`Role of ${m.name}`} value={m.role} disabled={update.isPending}
            onChange={(e) => { update.mutate({ role: e.target.value as Schemas["Role"] }); }}>
            {roles(myRole).map((r) => <option key={r} value={r}>{ROLE[r]}</option>)}
          </select>
        )}
      </td>
      <td><span className={`badge ${m.status === "active" ? "accepted" : m.status === "invited" ? "draft" : "withdrawn"}`}>{TEAM_STATUS[m.status]}</span></td>
      <td className="muted">{m.lastSignInAt ? formatWhen(m.lastSignInAt) : "Never"}</td>
      <td>
        {!locked && m.status === "active" && (
          <button type="button" className="link" onClick={() => { if (window.confirm(`Deactivate ${m.name}? They are signed out and cannot sign in.`)) update.mutate({ active: false }); }}>Deactivate</button>
        )}
        {!locked && m.status === "deactivated" && <button type="button" className="link" onClick={() => { update.mutate({ active: true }); }}>Reactivate</button>}
        {m.status === "invited" && <button type="button" className="link" disabled={resend.isPending} onClick={() => { resend.mutate(); }}>Send new invitation</button>}
        {link && <div className="secret">{link}</div>}
        <ErrorMessage error={update.error ?? resend.error} />
      </td>
    </tr>
  );
}

function Invite({ myRole }: { myRole: Schemas["Role"] }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<{ email: string; name: string; role: Schemas["Role"] }>({ email: "", name: "", role: "recruiter" });
  const [sent, setSent] = useState<Schemas["Invitation"] | null>(null);
  const invite = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/team/invites", { body: { ...form, email: form.email.trim() } })),
    onSuccess: async (inv) => {
      setSent(inv);
      setForm({ email: "", name: "", role: "recruiter" });
      await queryClient.invalidateQueries({ queryKey: teamKey });
    },
  });
  return (
    <section className="panel" aria-label="Invite someone">
      <h3>Invite someone</h3>
      {sent && (
        <div className="warning" role="status">
          <p>Invitation emailed to {sent.member.email}. If email is not set up, send them this link (it works for 7 days):</p>
          <code className="secret">{sent.link}</code>{" "}
          <button type="button" className="link" onClick={() => { setSent(null); }}>Done</button>
        </div>
      )}
      <form className="form-grid" onSubmit={(e) => { e.preventDefault(); invite.mutate(); }}>
        <label className="field">Email<input type="email" value={form.email} onChange={(e) => { setForm({ ...form, email: e.target.value }); }} /></label>
        <label className="field">Name<input value={form.name} onChange={(e) => { setForm({ ...form, name: e.target.value }); }} /></label>
        <label className="field">Role
          <select value={form.role} onChange={(e) => { setForm({ ...form, role: e.target.value as Schemas["Role"] }); }}>
            {roles(myRole).map((r) => <option key={r} value={r}>{ROLE[r]}</option>)}
          </select>
        </label>
        <SaveError error={invite.error} shown={[]} onReload={() => undefined} />
        <div className="row full"><button type="submit" disabled={invite.isPending || !form.email.trim() || !form.name.trim()}>Invite</button></div>
      </form>
    </section>
  );
}

/** Change your own password (your other sessions are signed out). */
export function ChangePassword() {
  const [form, setForm] = useState({ currentPassword: "", newPassword: "" });
  const [done, setDone] = useState(false);
  const change = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/me/password", { body: form })),
    onSuccess: (s) => {
      setToken(s.token); // this browser's new session
      setForm({ currentPassword: "", newPassword: "" });
      setDone(true);
    },
  });
  return (
    <section className="panel" aria-label="Your password">
      <h3>Your password</h3>
      {done && <p role="status">Password changed. Your other sessions are signed out.</p>}
      <form className="form-grid" onSubmit={(e) => { e.preventDefault(); setDone(false); change.mutate(); }}>
        <label className="field">Current password<input type="password" autoComplete="current-password" value={form.currentPassword} onChange={(e) => { setForm({ ...form, currentPassword: e.target.value }); }} /></label>
        <label className="field">New password (at least 10 characters)<input type="password" autoComplete="new-password" value={form.newPassword} onChange={(e) => { setForm({ ...form, newPassword: e.target.value }); }} /></label>
        <SaveError error={change.error} shown={[]} onReload={() => undefined} />
        <div className="row full"><button type="submit" disabled={change.isPending || !form.currentPassword || form.newPassword.length < 10}>Change password</button></div>
      </form>
    </section>
  );
}
