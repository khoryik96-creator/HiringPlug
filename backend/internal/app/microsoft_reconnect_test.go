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
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (f *msTest) sendReconnect(token, userID string) *response {
	f.t.Helper()
	return f.a.request(http.MethodPost, "/v1/team/"+userID+"/microsoft/reconnect", token, nil)
}

// reconnectLink is the token of a reconnect link an owner (or admin) sent.
func (f *msTest) reconnectLink(token, userID string) string {
	f.t.Helper()
	res := f.sendReconnect(token, userID)
	require.Equal(f.t, http.StatusCreated, res.Status, "%s", res.Body)
	var l api.MicrosoftReconnectLink
	res.JSON(&l)
	assert.True(f.t, l.Emailed)
	require.True(f.t, strings.HasPrefix(l.Link, "http://app.test/#/reconnect/"), l.Link)
	return l.Link[strings.LastIndex(l.Link, "/")+1:]
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
	link := f.reconnectLink(admin.Token, recruiter.ID)
	assert.Contains(t, strings.ReplaceAll(f.lastMailTo(tenant.ID, recruiterEmail), "=\r\n", ""), "#/reconnect/"+link) // quoted-printable
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

	// A link that expired, or is used meanwhile, does nothing.
	link = f.reconnectLink(owner.Token, owner.ID)
	flow, _ = f.start(map[string]any{"invite": link})
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE user_link SET expires_at = now() - interval '1 minute' WHERE user_id = $1 AND kind = 'reconnect'`, owner.ID)
	require.NoError(t, err)
	msRefused(t, f.signInAs(flow, newAccount(org, "boss."+tenant.ID+"@contoso.test").claims(flow)), "failed")
	assert.False(t, f.me(owner.Token).MicrosoftLinked)
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
}
