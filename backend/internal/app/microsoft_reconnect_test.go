package app_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (f *msTest) sendReconnect(token, userID string) *response {
	f.t.Helper()
	return f.a.request(http.MethodPost, "/v1/team/"+userID+"/microsoft/reconnect", token, nil)
}

var reconnectRe = regexp.MustCompile(`http://app\.test/#/reconnect/([0-9a-f]{64})`)

// reconnectLink is the token of a reconnect link an owner (or admin) sent:
// from the email to the person (whoever sends it never sees it).
func (f *msTest) reconnectLink(token, userID string) string {
	f.t.Helper()
	res := f.sendReconnect(token, userID)
	require.Equal(f.t, http.StatusCreated, res.Status, "%s", res.Body)
	var l api.MicrosoftReconnectLink
	res.JSON(&l)
	assert.True(f.t, l.Emailed)
	assert.NotContains(f.t, string(res.Body), "reconnect/", "the link is not shown to whoever sends it")
	var tenantID, email string
	require.NoError(f.t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT tenant_id, email FROM app_user WHERE id = $1`, userID).Scan(&tenantID, &email))
	mail := strings.ReplaceAll(f.lastMailTo(tenantID, email), "=\r\n", "") // quoted-printable
	m := reconnectRe.FindAllStringSubmatch(mail, -1)
	require.NotEmpty(f.t, m, "the link is emailed to them")
	return m[len(m)-1][1]
}

// Someone whose Microsoft sign-in name is not their HiringPlug email (or
// whose account was unlinked) is not locked out once Microsoft is required:
// an owner sends them a link, and signing in with Microsoft from it links
// the account to them, whatever its address. Once, for 7 days, an account
// of the company's organisation linked to nobody else there.
func TestReconnectLinks(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	recruiterEmail := emailOf(api.RoleRecruiter, tenant.ID)
	sam := newAccount(org, "sam.upn."+tenant.ID+"@contoso.test") // the sign-in name is not their email
	flow, _ := f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, sam.claims(flow)), "not_on_team")

	// Who may send one: owners and admins (only owners, to owners).
	assert.Equal(t, http.StatusForbidden, f.sendReconnect(recruiter.Token, recruiter.ID).Status)
	res := f.sendReconnect(admin.Token, owner.ID)
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeTEAMCHANGENOTALLOWED, res.Error().Error.Code)
	res = f.sendReconnect(admin.Token, admin.ID)
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "yourself: from Your account (with your password): %s", res.Body)
	link := f.reconnectLink(admin.Token, recruiter.ID)
	assert.Len(t, audited(t, tenant.ID, "user.microsoft_reconnect_sent"), 1)

	// The link is for Microsoft only: no password is chosen with it.
	var info api.AuthLink
	f.a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).JSON(&info)
	assert.Equal(t, api.AuthLinkKindReconnect, info.Kind)
	assert.Equal(t, api.AuthLinkMicrosoftRequired, info.Microsoft)
	assert.Equal(t, http.StatusGone, f.a.request(http.MethodPost, "/v1/auth/links/"+link, "", map[string]any{"password": "a-long-new-password-1"}).Status)

	// Another organisation's account: refused (the link is kept).
	flow, res = f.start(map[string]any{"invite": link})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, org, flow.authority, "the company's organisation")
	msRefused(t, f.signInAs(flow, newAccount(newGUID(), recruiterEmail).claims(flow)), "other_organisation")

	// Their own account, whatever its address: linked to them, and signed in.
	flow, _ = f.start(map[string]any{"invite": link})
	token := session(t, f.signInAs(flow, sam.claims(flow)))
	assert.Equal(t, recruiter.ID, f.me(token).User.Id)
	linked := audited(t, tenant.ID, "user.microsoft_linked")
	assert.Equal(t, []string{`{"by": "reconnect_link", "organisationId": "` + org + `"}`}, linked)
	assert.Contains(t, f.lastMailTo(tenant.ID, recruiterEmail), "with the link an owner sent you")
	flow, _ = f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow))) // from now on, as any sign-in

	// Once only.
	_, res = f.start(map[string]any{"invite": link})
	assert.Equal(t, http.StatusGone, res.Status, "%s", res.Body)

	// An account linked to someone else is not linked to another by a link.
	adminLink := f.reconnectLink(owner.Token, admin.ID)
	flow, _ = f.start(map[string]any{"invite": adminLink})
	msRefused(t, f.signInAs(flow, sam.claims(flow)), "linked_elsewhere")
	flow, _ = f.start(map[string]any{"invite": adminLink})
	assert.Equal(t, admin.ID, f.me(session(t, f.signInAs(flow, newAccount(org, "ada."+tenant.ID+"@contoso.test").claims(flow)))).User.Id,
		"the link still works for their own account")

	// Unlinked, they are linked again only on purpose: a new link does it.
	require.Equal(t, http.StatusNoContent, f.unlink(owner.Token, recruiter.ID).Status)
	flow, _ = f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, sam.claims(flow)), "not_on_team")
	link = f.reconnectLink(owner.Token, recruiter.ID)
	flow, _ = f.start(map[string]any{"invite": link})
	assert.Equal(t, recruiter.ID, f.me(session(t, f.signInAs(flow, sam.claims(flow)))).User.Id)

	// A link that expired meanwhile does nothing.
	require.Equal(t, http.StatusNoContent, f.unlink(owner.Token, recruiter.ID).Status)
	link = f.reconnectLink(admin.Token, recruiter.ID)
	flow, _ = f.start(map[string]any{"invite": link})
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE user_link SET expires_at = now() - interval '1 minute' WHERE user_id = $1 AND kind = 'reconnect'`, recruiter.ID)
	require.NoError(t, err)
	msRefused(t, f.signInAs(flow, sam.claims(flow)), "failed")
	assert.Empty(t, f.methodsOf(owner.Token, recruiter.ID))
}

// An invitation accepted with Microsoft links the account to the person
// invited, whatever its address (an account of the company's organisation).
func TestAnInvitationLinksWhateverTheAddress(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	f.enable(owner, org, true)
	res := f.a.request(http.MethodPost, "/v1/team/invites", owner, map[string]any{"email": "pat." + tenant.ID + "@test.local", "name": "Pat", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var inv api.Invitation
	res.JSON(&inv)
	invite := regexp.MustCompile(`[0-9a-f]{64}$`).FindString(inv.Link)

	flow, _ := f.start(map[string]any{"invite": invite})
	token := session(t, f.signInAs(flow, newAccount(org, "p.smith."+tenant.ID+"@contoso.test").claims(flow)))
	assert.Equal(t, inv.Member.Id, f.me(token).User.Id)
	assert.Equal(t, []string{`{"by": "invitation", "organisationId": "` + org + `"}`}, audited(t, tenant.ID, "user.microsoft_linked"))
}

// Someone made an owner keeps no Microsoft account linked by a sign-in's
// address (owners link one only on purpose): it is unlinked and their
// Microsoft sessions end. One they connected on purpose stays.
func TestSomeoneMadeAnOwnerKeepsNoAddressLink(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	flow, _ := f.start(map[string]any{"company": slug})
	samSession := session(t, f.signInAs(flow, sam.claims(flow))) // linked by its address
	ada := newAccount(org, "ada."+tenant.ID+"@contoso.test")
	require.Equal(t, http.StatusNoContent, f.connect(admin.Token, ada.claims).Status) // on purpose

	promote := func(userID string) {
		t.Helper()
		res := f.a.request(http.MethodPatch, "/v1/team/"+userID, owner.Token, map[string]any{"role": "owner"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}
	promote(recruiter.ID)
	assert.Empty(t, f.methodsOf(owner.Token, recruiter.ID), "the address link is gone")
	assert.Equal(t, http.StatusUnauthorized, status(f.a, samSession), "their Microsoft session ended")
	assert.Equal(t, []string{`{"reason": "made_owner", "organisationId": "` + org + `"}`}, audited(t, tenant.ID, "user.microsoft_unlinked"))
	flow, _ = f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, sam.claims(flow)), "connect_first")

	promote(admin.ID)
	assert.Equal(t, []api.SignInMethod{api.SignInMethodMicrosoft}, f.methodsOf(owner.Token, admin.ID), "connected on purpose: kept")
	flow, _ = f.start(map[string]any{"company": slug})
	assert.Equal(t, admin.ID, f.me(session(t, f.signInAs(flow, ada.claims(flow)))).User.Id)

	// No longer an owner, they still link one only on purpose.
	res := f.a.request(http.MethodPatch, "/v1/team/"+recruiter.ID, owner.Token, map[string]any{"role": "recruiter"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	flow, _ = f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, sam.claims(flow)), "connect_first")
	assert.Empty(t, f.methodsOf(owner.Token, recruiter.ID))
}

// Someone let to manage the team (not an admin) neither sends an admin a
// reconnect link (whoever holds it signs in as the admin) nor unlinks an
// admin's Microsoft account: nobody acts on a role above their own.
func TestReconnectLinksAndUnlinksKeepToTheirRank(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter, api.RoleSourcer)
	owner, admin, recruiter, sourcer := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer]
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	perms := append(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionIdentityTeamManage)
	f.a.setRole(owner.Token, "recruiter", map[string]any{"version": 0, "permissions": perms, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	for _, role := range []api.Role{api.RoleAdmin, api.RoleSourcer} { // both linked by their address
		flow, _ := f.start(map[string]any{"company": slug})
		session(t, f.signInAs(flow, newAccount(org, emailOf(role, tenant.ID)).claims(flow)))
	}
	teamRefused := func(res *response) {
		t.Helper()
		require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
		assert.Equal(t, api.ErrorCodeTEAMCHANGENOTALLOWED, res.Error().Error.Code)
	}

	teamRefused(f.sendReconnect(recruiter.Token, admin.ID))
	teamRefused(f.unlink(recruiter.Token, admin.ID))
	assert.Equal(t, []api.SignInMethod{api.SignInMethodMicrosoft}, f.methodsOf(owner.Token, admin.ID))
	assert.Empty(t, audited(t, tenant.ID, "user.microsoft_reconnect_sent"))
	// A teammate's: yes.
	assert.Equal(t, http.StatusCreated, f.sendReconnect(recruiter.Token, sourcer.ID).Status)
	assert.Equal(t, http.StatusNoContent, f.unlink(recruiter.Token, sourcer.ID).Status)
}

// A reconnect link is for who the person was when it was sent, until a
// Microsoft account is linked to them: unlinking theirs, changing their role
// (making them an owner too), deactivating them, linking one, or the company
// turning Microsoft sign-in off or changing its organisation ends it.
func TestReconnectLinksEndWhenThingsChange(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	setPassword(t, recruiter.ID, testPassword)
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	flow, _ := f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow))) // linked
	works := func(link string) int {
		return f.a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).Status
	}
	role := func(r string) {
		t.Helper()
		res := f.a.request(http.MethodPatch, "/v1/team/"+recruiter.ID, owner.Token, map[string]any{"role": r})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}

	link := f.reconnectLink(owner.Token, recruiter.ID)
	require.Equal(t, http.StatusOK, works(link))
	require.Equal(t, http.StatusNoContent, f.unlink(owner.Token, recruiter.ID).Status)
	assert.Equal(t, http.StatusGone, works(link), "unlinked")

	link = f.reconnectLink(owner.Token, recruiter.ID)
	role("sourcer")
	assert.Equal(t, http.StatusGone, works(link), "another role")

	link = f.reconnectLink(owner.Token, recruiter.ID)
	role("owner")
	assert.Equal(t, http.StatusGone, works(link), "made an owner")
	role("recruiter")

	link = f.reconnectLink(owner.Token, recruiter.ID)
	_, me := f.a.signIn(map[string]any{"email": sam.email, "password": testPassword})
	require.Equal(t, http.StatusNoContent, f.connect(me, sam.claims, map[string]any{"password": testPassword}).Status)
	assert.Equal(t, http.StatusGone, works(link), "connected from Your account")

	link = f.reconnectLink(owner.Token, recruiter.ID)
	for _, active := range []bool{false, true} {
		res := f.a.request(http.MethodPatch, "/v1/team/"+recruiter.ID, owner.Token, map[string]any{"active": active})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}
	assert.Equal(t, http.StatusGone, works(link), "deactivated (even if reactivated since)")

	link = f.reconnectLink(owner.Token, recruiter.ID)
	res := f.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", owner.Token, map[string]any{"enabled": false, "organisationId": org, "required": false})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	f.enable(owner.Token, org, false)
	assert.Equal(t, http.StatusGone, works(link), "Microsoft sign-in turned off (even if on again since)")

	link = f.reconnectLink(owner.Token, recruiter.ID)
	f.enable(owner.Token, newGUID(), false)
	assert.Equal(t, http.StatusGone, works(link), "another organisation")
}
