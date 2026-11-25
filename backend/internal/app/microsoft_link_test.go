package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra/entratest"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// lastMailTo is the body of the last email to address, once the queue ran.
func (f *msTest) lastMailTo(tenantID, address string) string {
	f.t.Helper()
	runCoreJobsUntilIdle(f.t, f.a, tenantID)
	msgs := mailServer.To(address)
	require.NotEmpty(f.t, msgs, "an email to %s", address)
	return msgs[len(msgs)-1].Data
}

// An administrator can type anyone's address as an account's email (mail);
// only the sign-in name, or an email Microsoft vouches for (xms_edov),
// recognises a team member. Guests are never accepted.
func TestMicrosoftTrustsTheSignInNameNotAnyEmail(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	org := newGUID()
	f.enable(tenant.Users[api.RoleOwner].Token, org, false)
	slug := f.me(tenant.Users[api.RoleOwner].Token).Tenant.CareersSlug
	recruiterEmail := emailOf(api.RoleRecruiter, tenant.ID)
	impostor := newAccount(org, "impostor."+tenant.ID+"@test.local")
	withMail := func(flow msFlow) jwt.MapClaims {
		c := impostor.claims(flow)
		c["email"] = recruiterEmail // their mail, set to the recruiter's address
		return c
	}

	flow, _ := f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, withMail(flow)), "not_on_team")
	flow, _ = f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, entratest.VerifiedEmail(withMail(flow), false)), "not_on_team")
	assert.Empty(t, audited(t, tenant.ID, "user.microsoft_linked"), "nobody was linked")

	// Microsoft vouches for the email's domain: it counts.
	sam := newAccount(org, "sam.upn."+tenant.ID+"@test.local")
	flow, _ = f.start(map[string]any{"company": slug})
	c := sam.claims(flow)
	c["email"] = recruiterEmail
	token := session(t, f.signInAs(flow, entratest.VerifiedEmail(c, true)))
	assert.Equal(t, tenant.Users[api.RoleRecruiter].ID, f.me(token).User.Id)

	// Guests: refused, said by the acct claim or by their sign-in name.
	guest := newAccount(org, emailOf(api.RoleOwner, tenant.ID))
	for _, withAcct := range []bool{true, false} {
		flow, _ = f.start(map[string]any{"company": slug})
		msRefused(t, f.signInAs(flow, entratest.Guest(guest.claims(flow), withAcct, true)), "other_organisation")
	}
	assert.Len(t, audited(t, tenant.ID, "user.microsoft_linked"), 1)
}

// An account is never linked to an owner by its address: they connect it
// from Your account while signed in (or accept their invitation with it).
// Whoever gets one linked is emailed.
func TestOwnersConnectMicrosoftFromTheirAccount(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	ownerEmail := emailOf(api.RoleOwner, tenant.ID)
	ana := newAccount(org, ownerEmail) // the owner's address as sign-in name

	flow, _ := f.start(map[string]any{"company": slug})
	e := msRefused(t, f.signInAs(flow, ana.claims(flow)), "connect_first")
	assert.Contains(t, e.Error.Message, "Connect Microsoft sign-in")
	assert.Empty(t, audited(t, tenant.ID, "user.microsoft_linked"))
	me := f.me(owner.Token)
	assert.True(t, me.MicrosoftSignIn)
	assert.False(t, me.MicrosoftLinked)

	// Another organisation's account, or a guest, cannot be connected.
	other := newAccount(newGUID(), ownerEmail)
	connectRefused(t, f.connect(owner.Token, func(fl msFlow) jwt.MapClaims {
		c := other.claims(fl)
		c["iss"] = entra.Issuer(other.org)
		return c
	}), "other_organisation")
	connectRefused(t, f.connect(owner.Token, func(fl msFlow) jwt.MapClaims { return entratest.Guest(ana.claims(fl), true, true) }), "other_organisation")

	// Connected from Your account: any address, audited, emailed.
	res := f.connect(owner.Token, func(fl msFlow) jwt.MapClaims {
		c := ana.claims(fl)
		c["preferred_username"], c["email"] = "ana.admin@contoso.test", "ana.admin@contoso.test"
		return c
	})
	require.Equal(t, http.StatusNoContent, res.Status, "%s", res.Body)
	assert.Equal(t, []string{`{"by": "self", "organisationId": "` + org + `"}`}, audited(t, tenant.ID, "user.microsoft_linked"))
	mail := f.lastMailTo(tenant.ID, ownerEmail)
	assert.Contains(t, mail, "ana.admin@contoso.test")
	assert.Contains(t, mail, "Not you? Tell an owner")
	assert.True(t, f.me(owner.Token).MicrosoftLinked)
	flow, _ = f.start(map[string]any{"company": slug})
	assert.Equal(t, owner.ID, f.me(session(t, f.signInAs(flow, ana.claims(flow)))).User.Id, "the owner signs in with it now")

	// Once only; and an account connected to someone is not connected to another.
	_, res = f.startConnecting(owner.Token)
	connectRefused(t, res, "already_connected")
	connectRefused(t, f.connect(recruiter.Token, ana.claims), "linked_elsewhere")

	// A connection is finished by the person who started it, signed in, in that tab only.
	flow, res = f.startConnecting(recruiter.Token)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	code := "code-" + randomBinding()
	f.ms.Grant(code, entratest.Grant{Claims: sam.claims(flow)})
	ticket := f.ticket(flow, code)
	msRefused(t, f.complete(ticket, flow.binding, ""), "failed") // not a sign-in: the ticket is gone
	flow, _ = f.startConnecting(recruiter.Token)
	f.ms.Grant(code+"b", entratest.Grant{Claims: sam.claims(flow)})
	connectRefused(t, f.finishConnecting(owner.Token, f.ticket(flow, code+"b"), flow.binding), "failed") // someone else
	flow, _ = f.start(map[string]any{"company": slug})
	f.ms.Grant(code+"c", entratest.Grant{Claims: sam.claims(flow)})
	connectRefused(t, f.finishConnecting(recruiter.Token, f.ticket(flow, code+"c"), flow.binding), "failed") // a sign-in's ticket
	assert.Equal(t, http.StatusUnauthorized, f.a.request(http.MethodPost, "/v1/me/microsoft/start", "",
		map[string]any{"bindingHash": bindingHash(randomBinding())}).Status, "signed in only")

	// A team member's first sign-in links their account: they are emailed too.
	flow, _ = f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow)))
	assert.Contains(t, f.lastMailTo(tenant.ID, sam.email), "Not you? Tell an owner")
}

// An owner invited accepts with Microsoft through their own invitation.
func TestAnInvitedOwnerAcceptsWithMicrosoft(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	f.enable(owner, org, false)
	email := "partner." + tenant.ID + "@test.local"
	res := f.a.request(http.MethodPost, "/v1/team/invites", owner, map[string]any{"email": email, "name": "Pat", "role": "owner"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var inv api.Invitation
	res.JSON(&inv)
	pat := newAccount(org, email)

	flow, _ := f.start(map[string]any{"company": f.me(owner).Tenant.CareersSlug})
	msRefused(t, f.signInAs(flow, pat.claims(flow)), "connect_first") // not through the invitation
	flow, _ = f.start(map[string]any{"invite": inv.Link[strings.LastIndex(inv.Link, "/")+1:]})
	assert.Equal(t, inv.Member.Id, f.me(session(t, f.signInAs(flow, pat.claims(flow)))).User.Id)
	assert.Equal(t, []string{`{"by": "invitation", "organisationId": "` + org + `"}`}, audited(t, tenant.ID, "user.microsoft_linked"))
}

// An invitation accepted with Microsoft works as long as the invitation
// does (7 days), like choosing a password with it.
func TestAnInvitationExpiresForMicrosoftToo(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	f.enable(owner, org, false)
	slug := f.me(owner).Tenant.CareersSlug
	invite := func(name string) api.Invitation {
		res := f.a.request(http.MethodPost, "/v1/team/invites", owner, map[string]any{"email": name + "." + tenant.ID + "@test.local", "name": name, "role": "recruiter"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var inv api.Invitation
		res.JSON(&inv)
		return inv
	}
	late, prompt := invite("late"), invite("prompt")
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE user_link SET expires_at = now() - interval '1 day', created_at = now() - interval '8 days' WHERE user_id = $1`, late.Member.Id)
	require.NoError(t, err)

	flow, _ := f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, newAccount(org, late.Member.Email).claims(flow)), "not_on_team")
	flow, _ = f.start(map[string]any{"company": slug})
	assert.Equal(t, prompt.Member.Id, f.me(session(t, f.signInAs(flow, newAccount(org, prompt.Member.Email).claims(flow)))).User.Id)

	// A new invitation lets them in again.
	res := f.a.request(http.MethodPost, "/v1/team/"+late.Member.Id+"/invite", owner, nil)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	flow, _ = f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, newAccount(org, late.Member.Email).claims(flow)))
}

// Without the optional acct claim a guest cannot be told from a member (a
// guest's preferred_username is its home address, which may be anyone's):
// no account is linked by its address then, and Settings says to add the
// claim. Accounts linked already, and connecting on purpose, still work.
func TestNoAddressLinksWithoutTheAcctClaim(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin]
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	warned := func() bool {
		var s api.MicrosoftSignInSettings
		f.a.request(http.MethodGet, "/v1/agency/microsoft-sign-in", owner.Token, nil).JSON(&s)
		return s.AcctClaimMissing
	}
	recruiterEmail := emailOf(api.RoleRecruiter, tenant.ID)
	sam := newAccount(org, recruiterEmail)
	flow, _ := f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow))) // acct sent: linked by its address
	assert.False(t, warned())

	// A guest whose home address is the admin's, and no acct nor upn sent.
	guest := newAccount(org, emailOf(api.RoleAdmin, tenant.ID))
	flow, _ = f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, entratest.Guest(guest.claims(flow), false, false)), "not_on_team")
	assert.Len(t, audited(t, tenant.ID, "user.microsoft_linked"), 1, "nobody linked")
	assert.True(t, warned(), "Settings says to add the acct claim")

	// Linked already: signs in. Connecting on purpose: works.
	flow, _ = f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, entratest.WithoutAcct(sam.claims(flow))))
	ada := newAccount(org, "ada."+tenant.ID+"@contoso.test")
	require.Equal(t, http.StatusNoContent, f.connect(admin.Token, func(fl msFlow) jwt.MapClaims { return entratest.WithoutAcct(ada.claims(fl)) }).Status)

	// The claim added in Entra: the warning goes.
	flow, _ = f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow)))
	assert.False(t, warned())
}
