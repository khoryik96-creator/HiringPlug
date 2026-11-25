package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra/entratest"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestSigningInWithMicrosoft(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	org := newGUID()
	settings := f.enable(tenant.Users[api.RoleOwner].Token, org, false)
	assert.Equal(t, "http://app.test/v1/auth/microsoft/callback", settings.CallbackUrl)
	assert.Equal(t, api.MicrosoftSignInSettingsAppSourceServer, settings.AppSource)
	recruiter := tenant.Users[api.RoleRecruiter]
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))

	// The careers page address finds the company: its organisation, through the server's app.
	flow, _ := f.start(map[string]any{"email": strings.ToUpper(sam.email), "company": f.me(tenant.Users[api.RoleOwner].Token).Tenant.CareersSlug})
	assert.Equal(t, org, flow.authority)
	assert.Equal(t, sam.email, flow.loginHint, "the email is suggested to Microsoft")
	assert.Equal(t, "server-client", flow.clientID)
	assert.Equal(t, "http://app.test/v1/auth/microsoft/callback", flow.redirect)

	// The first sign-in links the account to the team member with its address.
	token := session(t, f.signInAs(flow, sam.claims(flow)))
	me := f.me(token)
	assert.Equal(t, recruiter.ID, me.User.Id)
	require.NotNil(t, me.SignInMethod)
	assert.Equal(t, api.SignInMethodMicrosoft, *me.SignInMethod)
	exchanges := f.ms.Exchanges()
	last := exchanges[len(exchanges)-1]
	assert.Equal(t, "/"+org+"/oauth2/v2.0/token", last.Path)
	assert.Equal(t, "server-secret", last.ClientSecret)
	assert.Equal(t, "http://app.test/v1/auth/microsoft/callback", last.Redirect)
	assert.Equal(t, flow.challenge, entratest.Challenge(last.Verifier), "PKCE: the verifier behind the challenge")
	assert.Equal(t, []string{`{"organisationId": "` + org + `"}`}, audited(t, tenant.ID, "user.microsoft_linked"))
	assert.Contains(t, audited(t, tenant.ID, "user.signed_in"), `{"method": "microsoft"}`)

	// Later sign-ins match the account, whatever its address has become.
	flow, _ = f.start(nil)
	claims := sam.claims(flow)
	claims["email"], claims["preferred_username"] = "renamed@elsewhere.test", "renamed@elsewhere.test"
	assert.Equal(t, recruiter.ID, f.me(session(t, f.signInAs(flow, claims))).User.Id)
	assert.Len(t, audited(t, tenant.ID, "user.microsoft_linked"), 1, "linked once")

	// The team list says how each member signs in.
	var team api.TeamList
	f.a.request(http.MethodGet, "/v1/team", tenant.Users[api.RoleOwner].Token, nil).JSON(&team)
	for _, m := range team.Items {
		if m.Id == recruiter.ID {
			assert.Equal(t, []api.SignInMethod{api.SignInMethodMicrosoft}, m.SignInMethods)
		}
	}

	t.Run("a ticket works once, in the tab that started the sign-in", func(t *testing.T) {
		flow, _ := f.start(map[string]any{"email": sam.email})
		code := "code-" + randomBinding()
		f.ms.Grant(code, entratest.Grant{Claims: sam.claims(flow)})
		ticket := f.ticket(flow, code)
		assert.Equal(t, "failed", f.back(url.Values{"code": {code}, "state": {flow.state}}).Query().Get("error"), "Microsoft's redirect is used once")
		msRefused(t, f.complete(ticket, randomBinding(), ""), "failed") // another browser
		msRefused(t, f.complete(ticket, flow.binding, ""), "failed")    // the ticket went with that try
		before := len(f.ms.Exchanges())

		flow, _ = f.start(map[string]any{"email": sam.email})
		f.ms.Grant(code+"2", entratest.Grant{Claims: sam.claims(flow)})
		ticket = f.ticket(flow, code+"2")
		session(t, f.complete(ticket, flow.binding, ""))
		msRefused(t, f.complete(ticket, flow.binding, ""), "failed") // replayed
		assert.Len(t, f.ms.Exchanges(), before+1, "a code is exchanged once")
	})

	t.Run("Microsoft's answers are never passed on", func(t *testing.T) {
		flow, _ := f.start(nil)
		u := f.back(url.Values{"error": {"access_denied"}, "error_description": {"<script>alert(1)</script>"}, "state": {flow.state}})
		assert.Equal(t, url.Values{"error": {"cancelled"}}, u.Query())
		u = f.back(url.Values{"error": {"server_error"}, "state": {flow.state}})
		assert.Equal(t, url.Values{"error": {"failed"}}, u.Query())
		u = f.back(url.Values{"code": {"x"}, "state": {"forged"}})
		assert.Equal(t, url.Values{"error": {"failed"}}, u.Query())
		u = f.back(url.Values{"code": {"never-granted"}, "state": {flow.state}})
		e := msRefused(t, f.complete(u.Query().Get("ticket"), flow.binding, ""), "failed")
		assert.NotContains(t, e.Error.Message, "AADSTS")
	})
}

func TestMicrosoftSignInChecksTheIDToken(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	org := newGUID()
	f.enable(tenant.Users[api.RoleOwner].Token, org, false)
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	slug := f.me(tenant.Users[api.RoleOwner].Token).Tenant.CareersSlug
	other := newGUID()
	stranger := entratest.NewKey(t, "test-key-1") // the published key's ID, signed with another key
	cases := []struct {
		name, reason string
		change       func(jwt.MapClaims)
		key          *entratest.Key
	}{
		{"another organisation", "other_organisation", func(c jwt.MapClaims) { c["tid"], c["iss"] = other, entra.Issuer(other) }, nil},
		{"a personal account", "personal_account", func(c jwt.MapClaims) { c["tid"], c["iss"] = entra.ConsumersTenant, entra.Issuer(entra.ConsumersTenant) }, nil},
		{"another app", "failed", func(c jwt.MapClaims) { c["aud"] = "someone-else" }, nil},
		{"another issuer", "failed", func(c jwt.MapClaims) { c["iss"] = entra.Issuer(other) }, nil},
		{"expired", "failed", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-10 * time.Minute).Unix() }, nil},
		{"another sign-in's nonce", "failed", func(c jwt.MapClaims) { c["nonce"] = randomBinding() }, nil},
		{"a bad signature", "failed", func(jwt.MapClaims) {}, &stranger},
	}
	for _, tc := range cases {
		flow, _ := f.start(map[string]any{"company": slug})
		claims := sam.claims(flow)
		tc.change(claims)
		code := "code-" + randomBinding()
		f.ms.Grant(code, entratest.Grant{Claims: claims, Key: tc.key})
		msRefused(t, f.complete(f.ticket(flow, code), flow.binding, ""), tc.reason)
	}
	assert.Empty(t, audited(t, tenant.ID, "user.microsoft_linked"), "nobody was linked")

	// A key Microsoft has not published is refused; a new one it has is fetched.
	flow, _ := f.start(map[string]any{"company": slug})
	unpublished := entratest.NewKey(t, "never-published")
	code := "code-" + randomBinding()
	f.ms.Grant(code, entratest.Grant{Claims: sam.claims(flow), Key: &unpublished})
	msRefused(t, f.complete(f.ticket(flow, code), flow.binding, ""), "failed")
	fetches := f.ms.KeyFetches()
	f.ms.Rotate("rotated-key")
	flow, _ = f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow)))
	assert.Equal(t, fetches+1, f.ms.KeyFetches(), "the keys were fetched again for the new key")
}

func TestMicrosoftSignInIsOnlyForTheTeam(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	slug := f.me(owner).Tenant.CareersSlug

	// Not set up for the company: said before Microsoft, for its address only.
	_, res := f.start(map[string]any{"company": slug})
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMICROSOFTSIGNINUNAVAILABLE, res.Error().Error.Code)
	f.enable(owner, org, false)

	// Someone of the organisation who is not on the team: the redirect says
	// nothing either way, and the refusal is the one a deactivated member gets.
	flow, _ := f.start(map[string]any{"company": slug})
	notOnTeam := msRefused(t, f.signInAs(flow, newAccount(org, "nobody."+tenant.ID+"@test.local").claims(flow)), "not_on_team")
	assert.Contains(t, notOnTeam.Error.Message, "Ask an owner to invite you")

	admin := tenant.Users[api.RoleAdmin]
	adminAccount := newAccount(org, emailOf(api.RoleAdmin, tenant.ID))
	res = f.a.request(http.MethodPatch, "/v1/team/"+admin.ID, owner, map[string]any{"active": false})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	flow, _ = f.start(map[string]any{"company": slug})
	deactivated := msRefused(t, f.signInAs(flow, adminAccount.claims(flow)), "not_on_team")
	assert.Equal(t, notOnTeam.Error.Message, deactivated.Error.Message)

	// Linked, then deactivated: the account no longer signs in, nor do its sessions work.
	recruiter := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	flow, _ = f.start(map[string]any{"company": slug})
	token := session(t, f.signInAs(flow, recruiter.claims(flow)))
	res = f.a.request(http.MethodPatch, "/v1/team/"+tenant.Users[api.RoleRecruiter].ID, owner, map[string]any{"active": false})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusUnauthorized, f.a.request(http.MethodGet, "/v1/me", token, nil).Status)
	flow, _ = f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, recruiter.claims(flow)), "not_on_team")
}

func TestAcceptingAnInvitationWithMicrosoft(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	email := "new." + tenant.ID + "@test.local"
	res := f.a.request(http.MethodPost, "/v1/team/invites", owner, map[string]any{"email": email, "name": "Nadia", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var inv api.Invitation
	res.JSON(&inv)
	link := inv.Link[strings.LastIndex(inv.Link, "/")+1:]

	var info api.AuthLink
	f.a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).JSON(&info)
	assert.Equal(t, api.AuthLinkMicrosoftNone, info.Microsoft)
	f.enable(owner, org, false)
	f.a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).JSON(&info)
	assert.Equal(t, api.AuthLinkMicrosoftAvailable, info.Microsoft)

	_, res = f.start(map[string]any{"invite": strings.Repeat("0", 64)})
	assert.Equal(t, http.StatusGone, res.Status, "an unknown invitation")
	flow, _ := f.start(map[string]any{"invite": link})
	assert.Equal(t, org, flow.authority)
	token := session(t, f.signInAs(flow, newAccount(org, email).claims(flow)))
	me := f.me(token)
	assert.Equal(t, inv.Member.Id, me.User.Id)
	assert.False(t, me.HasPassword, "no password was chosen")
	assert.Equal(t, http.StatusGone, f.a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).Status, "the invitation is used")
	assert.Equal(t, []string{`{"method": "microsoft"}`}, audited(t, tenant.ID, "user.joined"))
	var team api.TeamList
	f.a.request(http.MethodGet, "/v1/team", owner, nil).JSON(&team)
	for _, m := range team.Items {
		if m.Id == inv.Member.Id {
			assert.Equal(t, api.TeamStatusActive, m.Status)
			assert.Equal(t, []api.SignInMethod{api.SignInMethodMicrosoft}, m.SignInMethods)
		}
	}

	// No password to change; two-step sign-in is turned off with the app's code instead.
	res = f.a.request(http.MethodPut, "/v1/me/password", token, map[string]any{"currentPassword": "", "newPassword": "a-new-password-123"})
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodePASSWORDNOTSET, res.Error().Error.Code)
	var setup api.TwoStepSetup
	f.a.request(http.MethodPost, "/v1/me/two-step/setup", token, nil).JSON(&setup)
	res = f.a.request(http.MethodPost, "/v1/me/two-step/confirm", token, map[string]any{"code": nextCode(t, setup.Secret, 0)})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res = f.a.request(http.MethodPost, "/v1/me/two-step/disable", token, map[string]any{"password": "anything-at-all"})
	assert.Equal(t, []string{"code"}, fieldNames(t, res.Error()))
	res = f.a.request(http.MethodPost, "/v1/me/two-step/disable", token, map[string]any{"code": nextCode(t, setup.Secret, 1)})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var status api.TwoStepStatus
	res.JSON(&status)
	assert.False(t, status.Enabled)
}

func TestMicrosoftSignInSkipsTheTwoStepCode(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	setPassword(t, recruiter.ID, testPassword)
	org := newGUID()
	f.enable(owner.Token, org, false)

	// The owner sets up an app and requires two-step sign-in of everyone.
	var setup api.TwoStepSetup
	f.a.request(http.MethodPost, "/v1/me/two-step/setup", owner.Token, nil).JSON(&setup)
	require.Equal(t, http.StatusOK, f.a.request(http.MethodPost, "/v1/me/two-step/confirm", owner.Token, map[string]any{"code": nextCode(t, setup.Secret, 0)}).Status)
	require.Equal(t, http.StatusOK, f.a.request(http.MethodPut, "/v1/agency/two-step", owner.Token, map[string]any{"required": true}).Status)

	// A password still needs the code (here: setting the app up first)...
	res, _ := f.a.signIn(map[string]any{"email": emailOf(api.RoleRecruiter, tenant.ID), "password": testPassword})
	challengeOf(t, res, api.ErrorCodeTWOSTEPSETUPREQUIRED)
	// ...Microsoft does not.
	flow, _ := f.start(map[string]any{"email": emailOf(api.RoleRecruiter, tenant.ID)})
	token := session(t, f.signInAs(flow, newAccount(org, emailOf(api.RoleRecruiter, tenant.ID)).claims(flow)))
	assert.Equal(t, recruiter.ID, f.me(token).User.Id)
}
