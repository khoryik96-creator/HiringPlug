import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { setToken } from "../auth";
import { teamsKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { Menu } from "../components/Menu";
import { SignInMethods } from "../components/SignInMethods";
import { SaveError } from "../components/SaveError";
import { formatWhen, keysOf, ROLE, TEAM_STATUS } from "../labels";
import { InvitationEmailState, InvitationSent } from "./TeamInvitations";

type Member = Schemas["TeamMember"];
type Group = Schemas["TeamGroup"];
const teamKey = ["team"] as const;

/** Who is in the company: invite people, change roles, deactivate (owners and admins). */
export function Team({ myRole, myId, microsoft = false }: { myRole: Schemas["Role"]; myId: string; microsoft?: boolean }) {
  const list = useQuery({ queryKey: teamKey, queryFn: async () => unwrap(await api.GET("/v1/team")).items });
  const groups = useQuery({ queryKey: teamsKey, queryFn: async () => unwrap(await api.GET("/v1/teams")).items });
  return (
    <main>
      <h2>Team</h2>
      {myRole === "owner" && <p className="muted small"><a href="#/access">Roles and access</a></p>}
      <ErrorMessage error={list.error ?? groups.error} />
      {list.data && (
        <div className="table-scroll">
          <table className="team-table">
            <thead><tr><th>Person</th><th>Role</th><th>Team</th><th>Status</th><th><span className="visually-hidden">Actions</span></th></tr></thead>
            <tbody>{list.data.map((m) => <MemberRow key={m.id} member={m} myRole={myRole} me={m.id === myId} groups={groups.data ?? []} microsoft={microsoft} />)}</tbody>
          </table>
        </div>
      )}
      <Teams groups={groups.data ?? []} />
      <Invite myRole={myRole} />
    </main>
  );
}

/** Owners above admins above everyone else: nobody gives or changes a role above their own. */
const rank = (r: Schemas["Role"]) => (r === "owner" ? 3 : r === "admin" ? 2 : 1);
const roles = (myRole: Schemas["Role"]) => keysOf(ROLE).filter((r) => rank(r) <= rank(myRole));

/** A link just emailed to someone: an invitation (shown, to pass on another way if email is not set up), or to connect Microsoft (never shown: it signs in as them). */
type Sent = { what: string; link?: string; delivery?: Schemas["Invitation"]["delivery"] };


function MemberRow({ member: m, myRole, me, groups, microsoft }: { member: Member; myRole: Schemas["Role"]; me: boolean; groups: Group[]; microsoft: boolean }) {
  const queryClient = useQueryClient();
  const [sent, setSent] = useState<Sent | null>(null);
  const refresh = async () => { await queryClient.invalidateQueries({ queryKey: teamKey }); };
  const update = useMutation({
    mutationFn: async (body: Schemas["TeamMemberUpdate"]) =>
      unwrap(await api.PATCH("/v1/team/{userId}", { params: { path: { userId: m.id } }, body })),
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey: teamKey });
      await queryClient.invalidateQueries({ queryKey: teamsKey });
    },
  });
  const resend = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/team/{userId}/invite", { params: { path: { userId: m.id } } })),
    onSuccess: (inv) => { setSent({ what: "New invitation emailed", link: inv.link, delivery: inv.delivery }); },
  });
  const reconnect = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/team/{userId}/microsoft/reconnect", { params: { path: { userId: m.id } } })),
    onSuccess: () => { setSent({ what: "Link to connect Microsoft emailed" }); },
  });
  const resetTwoStep = useMutation({
    mutationFn: async () => { unwrap(await api.DELETE("/v1/team/{userId}/two-step", { params: { path: { userId: m.id } } })); },
    onSuccess: refresh,
  });
  const unlink = useMutation({
    mutationFn: async () => { unwrap(await api.DELETE("/v1/team/{userId}/microsoft", { params: { path: { userId: m.id } } })); },
    onSuccess: refresh,
  });
  const cancel = useMutation({
    mutationFn: async () => { unwrap(await api.DELETE("/v1/team/{userId}/invite", { params: { path: { userId: m.id } } })); },
    onSuccess: refresh,
  });
  const locked = me || rank(m.role) > rank(myRole);
  const mayOwners = myRole === "owner" || m.role !== "owner"; // only owners act on owners' sign-in
  const linked = m.signInMethods.includes("microsoft");
  const actions: { label: string; danger?: boolean; busy?: boolean; run: () => void }[] = [];
  if (!locked && m.status === "active") {
    actions.push({ label: "Deactivate", run: () => { if (window.confirm(`Deactivate ${m.name}? They are signed out and cannot sign in.`)) update.mutate({ active: false }); } });
  }
  if (!locked && m.status === "deactivated") actions.push({ label: "Reactivate", run: () => { update.mutate({ active: true }); } });
  if (m.twoStep && !me && mayOwners) {
    actions.push({ label: "Reset two-step", busy: resetTwoStep.isPending, run: () => {
      if (window.confirm(`Reset ${m.name}'s two-step sign-in (a lost phone)? They are signed out and add their app again.`)) resetTwoStep.mutate();
    } });
  }
  if (linked && mayOwners) {
    actions.push({ label: "Unlink Microsoft", busy: unlink.isPending, run: () => {
      if (window.confirm(`Unlink ${m.name}'s Microsoft account? Their sessions signed in with Microsoft end now. `
        + "A Microsoft account is then connected to them only on purpose: from Your account, or with a link you send them (Reconnect Microsoft).")) unlink.mutate();
    } });
  }
  if (microsoft && !linked && !me && m.status === "active" && mayOwners) {
    actions.push({ label: "Reconnect Microsoft", busy: reconnect.isPending, run: () => {
      if (window.confirm(`Email ${m.name} a link to connect their Microsoft work account, whatever its address? `
        + "Signing in with Microsoft from it connects that account to them. The link works once, for 7 days.")) reconnect.mutate();
    } });
  }
  if (m.status === "invited") actions.push({ label: "Send new invitation", busy: resend.isPending, run: () => { resend.mutate(); } });
  if (m.status === "invited" && !locked) {
    actions.push({ label: "Cancel invitation", danger: true, busy: cancel.isPending, run: () => {
      if (window.confirm(`Cancel the invitation of ${m.name} (${m.email})? Their link stops working now; you can invite them again later.`)) cancel.mutate();
    } });
  }
  const error = update.error ?? resend.error ?? cancel.error ?? resetTwoStep.error ?? unlink.error ?? reconnect.error;
  return (
    <>
      <tr className={sent || error ? "team-row has-note" : "team-row"}>
        <td className="team-person">
          <div className="team-name">{m.name}{me && <span className="muted"> (you)</span>}</div>
          <div className="team-email muted">{m.email}</div>
          <SignInMethods methods={m.signInMethods} />
        </td>
        <td className="team-role">
          {locked ? <span className="team-role-text">{ROLE[m.role]}</span> : (
            <select aria-label={`Role of ${m.name}`} value={m.role} disabled={update.isPending}
              onChange={(e) => { update.mutate({ role: e.target.value as Schemas["Role"] }); }}>
              {roles(myRole).map((r) => <option key={r} value={r}>{ROLE[r]}</option>)}
            </select>
          )}
        </td>
        <td className="team-group">
          <select aria-label={`Team of ${m.name}`} value={m.teamId ?? ""} disabled={update.isPending}
            onChange={(e) => { update.mutate({ teamId: e.target.value || null }); }}>
            <option value="">No team</option>
            {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
          </select>
        </td>
        <td className="team-status">
          <span className={`badge ${m.status === "active" ? "accepted" : m.status === "invited" ? "draft" : "withdrawn"}`}>{TEAM_STATUS[m.status]}</span>
          {m.twoStep && <span className="badge two-step-badge" title="Signs in with a code from an authenticator app too">2-step</span>}
          <div className="team-last muted small">{m.lastSignInAt ? <>Last signed in {formatWhen(m.lastSignInAt)}</> : "Never signed in"}</div>
          {m.status === "invited" && m.invitationEmail && <InvitationEmailState email={m.invitationEmail} />}
        </td>
        <td className="team-actions">
          {actions.length > 0 && (
            <Menu label="⋯" ariaLabel={`Actions for ${m.name}`}>
              {(close) => actions.map((a) => (
                <button key={a.label} type="button" className={a.danger ? "danger" : undefined} disabled={a.busy}
                  onClick={() => { close(); a.run(); }}>{a.label}</button>
              ))}
            </Menu>
          )}
        </td>
      </tr>
      {(sent || error) && (
        <tr className="team-note">
          <td colSpan={5}>
            {sent?.link && sent.delivery && <InvitationSent email={m.email} link={sent.link} delivery={sent.delivery} onDone={() => { setSent(null); }} />}
            {sent && !sent.link && (
              <div className="team-sent" role="status">
                {sent.what} to {m.email}. It works once, for 7 days.{" "}
                <button type="button" className="link" onClick={() => { setSent(null); }}>Done</button>
              </div>
            )}
            <ErrorMessage error={error} />
          </td>
        </tr>
      )}
    </>
  );
}

/** Teams of people (e.g. desks): roles set to see their team's candidates see their teammates'. */
function Teams({ groups }: { groups: Group[] }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: teamsKey });
    await queryClient.invalidateQueries({ queryKey: teamKey });
  };
  const add = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/teams", { body: { name: name.trim() } })),
    onSuccess: async () => { setName(""); await refresh(); },
  });
  const remove = useMutation({
    mutationFn: async (id: string) => { unwrap(await api.DELETE("/v1/teams/{teamId}", { params: { path: { teamId: id } } })); },
    onSuccess: refresh,
  });
  return (
    <section className="panel" aria-label="Teams">
      <h3>Teams</h3>
      <ul>
        {groups.map((g) => (
          <li key={g.id}>
            <TeamName group={g} onDone={refresh} /> <span className="muted">{g.memberCount} {g.memberCount === 1 ? "person" : "people"}</span>{" "}
            <button type="button" className="link" disabled={remove.isPending}
              onClick={() => { if (window.confirm(`Remove the team ${g.name}? Its people stay, in no team.`)) remove.mutate(g.id); }}>Remove</button>
          </li>
        ))}
      </ul>
      <form className="row" onSubmit={(e) => { e.preventDefault(); add.mutate(); }}>
        <input required aria-label="New team" placeholder="New team" maxLength={100} value={name} onChange={(e) => { setName(e.target.value); }} />
        <button type="submit" disabled={add.isPending}>Add team</button>
      </form>
      <ErrorMessage error={add.error ?? remove.error} />
    </section>
  );
}

function TeamName({ group, onDone }: { group: Group; onDone: () => Promise<void> }) {
  const [name, setName] = useState<string | null>(null);
  const rename = useMutation({
    mutationFn: async (to: string) => unwrap(await api.PATCH("/v1/teams/{teamId}", { params: { path: { teamId: group.id } }, body: { version: group.version, name: to } })),
    onSuccess: async () => { setName(null); await onDone(); },
  });
  if (name === null) return <button type="button" className="link" title="Rename" onClick={() => { setName(group.name); }}>{group.name}</button>;
  return (
    <form className="row" style={{ display: "inline-flex" }} onSubmit={(e) => { e.preventDefault(); rename.mutate(name.trim()); }}>
      <input required aria-label={`Rename ${group.name}`} value={name} maxLength={100} onChange={(e) => { setName(e.target.value); }} />
      <button type="submit" disabled={rename.isPending}>Rename</button>
      <button type="button" className="link" onClick={() => { setName(null); }}>Cancel</button>
      <ErrorMessage error={rename.error} />
    </form>
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
      {sent && <InvitationSent email={sent.member.email} link={sent.link} delivery={sent.delivery} onDone={() => { setSent(null); }} />}
      <form className="form-grid" onSubmit={(e) => { e.preventDefault(); invite.mutate(); }}>
        <label className="field">Email<input type="email" required value={form.email} onChange={(e) => { setForm({ ...form, email: e.target.value }); }} /></label>
        <label className="field">Name<input required value={form.name} onChange={(e) => { setForm({ ...form, name: e.target.value }); }} /></label>
        <label className="field">Role
          <select value={form.role} onChange={(e) => { setForm({ ...form, role: e.target.value as Schemas["Role"] }); }}>
            {roles(myRole).map((r) => <option key={r} value={r}>{ROLE[r]}</option>)}
          </select>
        </label>
        <SaveError error={invite.error} shown={[]} onReload={() => undefined} />
        <div className="row full"><button type="submit" disabled={invite.isPending}>Invite</button></div>
      </form>
    </section>
  );
}

/** Change your own password (your other sessions are signed out); none to change for Microsoft-only accounts. */
export function ChangePassword({ hasPassword = true }: { hasPassword?: boolean }) {
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
  if (!hasPassword) {
    return (
      <section className="panel" aria-label="Your password">
        <h3>Your password</h3>
        <p>You sign in with Microsoft, so there is no HyPlug password to change.</p>
      </section>
    );
  }
  return (
    <section className="panel" aria-label="Your password">
      <h3>Your password</h3>
      {done && <p role="status">Password changed. Your other sessions are signed out.</p>}
      <form className="form-grid" onSubmit={(e) => { e.preventDefault(); setDone(false); change.mutate(); }}>
        <label className="field">Current password<input type="password" required autoComplete="current-password" value={form.currentPassword} onChange={(e) => { setForm({ ...form, currentPassword: e.target.value }); }} /></label>
        <label className="field">New password (at least 10 characters)<input type="password" required minLength={10} autoComplete="new-password" value={form.newPassword} onChange={(e) => { setForm({ ...form, newPassword: e.target.value }); }} /></label>
        <SaveError error={change.error} shown={[]} onReload={() => undefined} />
        <div className="row full"><button type="submit" disabled={change.isPending}>Change password</button></div>
      </form>
    </section>
  );
}
