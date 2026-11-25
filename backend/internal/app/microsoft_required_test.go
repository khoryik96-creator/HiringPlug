package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestRequiringMicrosoft(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleAdmin)
	owner, recruiter, admin := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleAdmin]
	ownerEmail, recruiterEmail, adminEmail := emailOf(api.RoleOwner, tenant.ID), emailOf(api.RoleRecruiter, tenant.ID), emailOf(api.RoleAdmin, tenant.ID)
	for _, id := range []string{owner.ID, recruiter.ID, admin.ID} {
		setPassword(t, id, testPassword)
	}
	org := newGUID()
	f.enable(owner.Token, org, false)

	// Before: password and Microsoft sessions alike; a reset link made now.
	res, recruiterPassword := f.a.signIn(map[string]any{"email": recruiterEmail, "password": testPassword})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	_, ownerPassword := f.a.signIn(map[string]any{"email": ownerEmail, "password": testPassword})
	assert.Equal(t, api.SignInMethodPassword, *f.me(recruiterPassword).SignInMethod)
	flow, _ := f.start(map[string]any{"email": recruiterEmail})
	recruiterMicrosoft := session(t, f.signInAs(flow, newAccount(org, recruiterEmail).claims(flow)))
	res = f.a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": adminEmail})
	require.Equal(t, http.StatusAccepted, res.Status)
	runCoreJobsUntilIdle(t, f.a, tenant.ID)
	adminReset := linkIn(t, adminEmail)
	var setup api.TwoStepSetup // the admin has an authenticator app, and is half-way through signing in
	f.a.request(http.MethodPost, "/v1/me/two-step/setup", admin.Token, nil).JSON(&setup)
	require.Equal(t, http.StatusOK, f.a.request(http.MethodPost, "/v1/me/two-step/confirm", admin.Token, map[string]any{"code": nextCode(t, setup.Secret, 0)}).Status)
	res, _ = f.a.signIn(map[string]any{"email": adminEmail, "password": testPassword})
	challenge := challengeOf(t, res, api.ErrorCodeTWOSTEPREQUIRED)

	f.enable(owner.Token, org, true)
	assert.Equal(t, []string{
		`{"before": {"enabled": false, "required": false, "organisationId": null}, "enabled": true, "required": false, "organisationId": "` + org + `"}`,
		`{"before": {"enabled": true, "required": false, "organisationId": "` + org + `"}, "enabled": true, "required": true, "organisationId": "` + org + `"}`,
	}, audited(t, tenant.ID, "agency.microsoft_sign_in_changed"))

	// Password sessions end at once, except the owners'; Microsoft sessions go on.
	assert.Equal(t, http.StatusUnauthorized, f.a.request(http.MethodGet, "/v1/me", recruiterPassword, nil).Status)
	assert.Equal(t, http.StatusUnauthorized, f.a.request(http.MethodGet, "/v1/me", admin.Token, nil).Status, "a session that does not say how it was signed in")
	assert.Equal(t, http.StatusOK, f.a.request(http.MethodGet, "/v1/me", recruiterMicrosoft, nil).Status)
	assert.Equal(t, http.StatusOK, f.a.request(http.MethodGet, "/v1/me", ownerPassword, nil).Status)

	// Passwords are refused to everyone but owners (only once the password is right).
	res, _ = f.a.signIn(map[string]any{"email": recruiterEmail, "password": testPassword})
	require.Equal(t, http.StatusForbidden, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMICROSOFTREQUIRED, res.Error().Error.Code)
	res, _ = f.a.signIn(map[string]any{"email": recruiterEmail, "password": "a-wrong-password-1"})
	assert.Equal(t, api.ErrorCodeSIGNINFAILED, res.Error().Error.Code)
	res, _ = f.a.signIn(map[string]any{"email": ownerEmail, "password": testPassword})
	assert.Equal(t, http.StatusOK, res.Status, "owners keep their password")
	res = f.a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge, "code": nextCode(t, setup.Secret, 1)})
	require.Equal(t, http.StatusForbidden, res.Status, "a code does not finish a password sign-in begun before: %s", res.Body)
	assert.Equal(t, api.ErrorCodeMICROSOFTREQUIRED, res.Error().Error.Code)
	res = f.a.request(http.MethodPut, "/v1/me/password", recruiterMicrosoft, map[string]any{"currentPassword": testPassword, "newPassword": "a-new-password-123"})
	require.Equal(t, http.StatusForbidden, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMICROSOFTREQUIRED, res.Error().Error.Code)

	// Password resets: no link, but how to sign in; a link made before is refused.
	before := len(mailServer.To(recruiterEmail))
	res = f.a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": recruiterEmail})
	require.Equal(t, http.StatusAccepted, res.Status, "the same answer")
	runCoreJobsUntilIdle(t, f.a, tenant.ID)
	msgs := mailServer.To(recruiterEmail)
	require.Len(t, msgs, before+1)
	assert.Contains(t, msgs[len(msgs)-1].Data, "Sign in with Microsoft")
	assert.NotRegexp(t, linkRe, msgs[len(msgs)-1].Data)
	assert.Len(t, audited(t, tenant.ID, "user.reset_refused"), 1)
	res = f.a.request(http.MethodPost, "/v1/auth/links/"+adminReset, "", map[string]any{"password": "another-password-42"})
	require.Equal(t, http.StatusForbidden, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMICROSOFTREQUIRED, res.Error().Error.Code)
	res = f.a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": ownerEmail})
	require.Equal(t, http.StatusAccepted, res.Status)
	runCoreJobsUntilIdle(t, f.a, tenant.ID)
	assert.NotEmpty(t, linkIn(t, ownerEmail), "owners still get a link")

	// An invitation is accepted with Microsoft, not a password.
	res = f.a.request(http.MethodPost, "/v1/team/invites", owner.Token, map[string]any{"email": "later." + tenant.ID + "@test.local", "name": "Later", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var inv api.Invitation
	res.JSON(&inv)
	invite := inv.Link[strings.LastIndex(inv.Link, "/")+1:]
	var info api.AuthLink
	f.a.request(http.MethodGet, "/v1/auth/links/"+invite, "", nil).JSON(&info)
	assert.Equal(t, api.AuthLinkMicrosoftRequired, info.Microsoft)
	res = f.a.request(http.MethodPost, "/v1/auth/links/"+invite, "", map[string]any{"password": "another-password-42"})
	assert.Equal(t, api.ErrorCodeMICROSOFTREQUIRED, res.Error().Error.Code)
	flow, _ = f.start(map[string]any{"invite": invite})
	session(t, f.signInAs(flow, newAccount(org, "later."+tenant.ID+"@test.local").claims(flow)))

	// Stopping it lets passwords work again.
	f.enable(owner.Token, org, false)
	res, _ = f.a.signIn(map[string]any{"email": recruiterEmail, "password": testPassword})
	assert.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
}

func TestMicrosoftSignInSettingsAreForOwners(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	owner, admin := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token
	assert.Equal(t, http.StatusForbidden, f.a.request(http.MethodGet, "/v1/agency/microsoft-sign-in", admin, nil).Status)
	assert.Equal(t, http.StatusForbidden, f.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", admin, map[string]any{"enabled": false, "organisationId": nil, "required": false}).Status)

	var s api.MicrosoftSignInSettings
	res := f.a.request(http.MethodGet, "/v1/agency/microsoft-sign-in", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&s)
	assert.False(t, s.Enabled)
	assert.Equal(t, "server-client", s.ClientId.MustGet())

	for body, field := range map[*map[string]any]string{
		{"enabled": true, "organisationId": nil, "required": false}:                                    "organisationId",
		{"enabled": true, "organisationId": "contoso.com", "required": false}:                          "organisationId",
		{"enabled": true, "organisationId": "9188040d-6c67-4c5b-b112-36a304b66dad", "required": false}: "organisationId",
		{"enabled": false, "organisationId": newGUID(), "required": true}:                              "required",
	} {
		res := f.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", owner, *body)
		require.Equal(t, http.StatusBadRequest, res.Status, "%v: %s", *body, res.Body)
		assert.Equal(t, []string{field}, fieldNames(t, res.Error()), "%v", *body)
	}
	assert.Empty(t, audited(t, tenant.ID, "agency.microsoft_sign_in_changed"), "nothing was saved")

	org := newGUID()
	s = f.enable(owner, strings.ToUpper(org), false)
	assert.Equal(t, org, s.OrganisationId.MustGet(), "kept in normal form")
	f.enable(owner, org, false)
	assert.Len(t, audited(t, tenant.ID, "agency.microsoft_sign_in_changed"), 1, "saving the same again changes nothing")
}

func TestMicrosoftSignInAtSeveralCompanies(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	org := newGUID()
	email := "shared." + newGUID() + "@test.local"
	var slugs, users []string
	for range 2 {
		tenant := testutil.Current.NewTenant(t, api.RoleOwner)
		owner := tenant.Users[api.RoleOwner].Token
		f.enable(owner, org, false)
		uid, err := app.CreateUser(context.Background(), testutil.Current.Admin, tenant.ID, email, "Sam", api.RoleRecruiter)
		require.NoError(t, err)
		slugs, users = append(slugs, f.me(owner).Tenant.CareersSlug), append(users, uid)
	}

	// The email is at two companies: any company of the organisation, through the server's app.
	flow, _ := f.start(map[string]any{"email": email})
	assert.Equal(t, "organizations", flow.authority)
	assert.Equal(t, "server-client", flow.clientID)
	sam := newAccount(org, email)
	res := f.signInAs(flow, sam.claims(flow))
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	e := res.Error()
	assert.Equal(t, api.ErrorCodeSIGNINCHOOSECOMPANY, e.Error.Code)
	details := *e.Error.Details
	assert.Len(t, details["companies"], 2)
	choice, _ := details["ticket"].(string)
	require.NotEmpty(t, choice)

	msRefused(t, f.complete(choice, randomBinding(), slugs[1]), "failed") // another browser: the choice is gone
	flow, _ = f.start(map[string]any{"email": email})
	res = f.signInAs(flow, sam.claims(flow))
	choice, _ = (*res.Error().Error.Details)["ticket"].(string)
	token := session(t, f.complete(choice, flow.binding, slugs[1]))
	assert.Equal(t, users[1], f.me(token).User.Id)
	msRefused(t, f.complete(choice, flow.binding, slugs[0]), "failed") // the choice works once

	// The account is linked at the second company now; the first still matches by email.
	flow, _ = f.start(map[string]any{"company": slugs[0]})
	assert.Equal(t, users[0], f.me(session(t, f.signInAs(flow, sam.claims(flow)))).User.Id)
}

func TestMicrosoftSignInWithTheAgencysAppOrTheServers(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	agency := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	plain := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	agencyOrg, plainOrg := newGUID(), newGUID()
	res := f.a.request(http.MethodPut, "/v1/settings/email-apps/microsoft", agency.Users[api.RoleOwner].Token,
		map[string]any{"clientId": "agency-client", "clientSecret": "agency-secret-value"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	s := f.enable(agency.Users[api.RoleOwner].Token, agencyOrg, false)
	assert.Equal(t, api.MicrosoftSignInSettingsAppSourceAgency, s.AppSource)
	f.enable(plain.Users[api.RoleOwner].Token, plainOrg, false)

	// A company with its own app signs in through it, at its organisation.
	flow, _ := f.start(map[string]any{"company": f.me(agency.Users[api.RoleOwner].Token).Tenant.CareersSlug})
	assert.Equal(t, agencyOrg, flow.authority)
	assert.Equal(t, "agency-client", flow.clientID)
	session(t, f.signInAs(flow, newAccount(agencyOrg, emailOf(api.RoleRecruiter, agency.ID)).claims(flow)))
	ex := f.ms.Exchanges()
	assert.Equal(t, "agency-secret-value", ex[len(ex)-1].ClientSecret)

	// Naming no company (two sign in with Microsoft here): the server's app,
	// for any organisation; the account's organisation finds the company.
	flow, _ = f.start(nil)
	assert.Equal(t, "organizations", flow.authority)
	assert.Equal(t, "server-client", flow.clientID)
	token := session(t, f.signInAs(flow, newAccount(plainOrg, emailOf(api.RoleRecruiter, plain.ID)).claims(flow)))
	assert.Equal(t, plain.Users[api.RoleRecruiter].ID, f.me(token).User.Id)
	ex = f.ms.Exchanges()
	assert.Equal(t, "/organizations/oauth2/v2.0/token", ex[len(ex)-1].Path)
	assert.Equal(t, "server-secret", ex[len(ex)-1].ClientSecret)

	// A token for the server's app is not accepted for a sign-in through the agency's.
	flow, _ = f.start(map[string]any{"company": f.me(agency.Users[api.RoleOwner].Token).Tenant.CareersSlug})
	claims := newAccount(agencyOrg, emailOf(api.RoleRecruiter, agency.ID)).claims(flow)
	claims["aud"] = "server-client"
	msRefused(t, f.signInAs(flow, claims), "failed")
}

// Not parallel: it needs to be the only company on the server that signs in
// with Microsoft (parallel tests start only after it).
func TestMicrosoftSignInGoesToTheOnlyCompanyThatUsesIt(t *testing.T) {
	f := newMicrosoftTest(t)
	// Left on by earlier runs (go test -count=2): no test is running meanwhile.
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE tenant SET microsoft_required = false, microsoft_sign_in = false WHERE microsoft_sign_in`)
	require.NoError(t, err)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	f.enable(owner, org, false)
	t.Cleanup(func() {
		f.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", owner, map[string]any{"enabled": false, "organisationId": org, "required": false})
	})
	for _, body := range []map[string]any{nil, {"email": "unknown." + tenant.ID + "@test.local"}} {
		flow, _ := f.start(body)
		assert.Equal(t, org, flow.authority, "an unknown email goes where none would")
	}
	flow, _ := f.start(nil)
	session(t, f.signInAs(flow, newAccount(org, emailOf(api.RoleRecruiter, tenant.ID)).claims(flow)))
}

// The email typed never chooses the company: the address Microsoft is
// opened at (organisation, app) would show where it is on a team.
func TestTheEmailDoesNotShowWhereSomeoneIsOnATeam(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	at := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	org := newGUID()
	f.enable(at.Users[api.RoleOwner].Token, org, false)
	f.enable(other.Users[api.RoleOwner].Token, newGUID(), false) // two companies sign in with Microsoft
	email := emailOf(api.RoleRecruiter, at.ID)
	for _, typed := range []string{email, "nobody." + at.ID + "@test.local"} {
		flow, res := f.start(map[string]any{"email": typed})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, "organizations", flow.authority, typed)
		assert.Equal(t, "server-client", flow.clientID, typed)
	}
	// The account's organisation still finds the company.
	flow, _ := f.start(map[string]any{"email": email})
	assert.Equal(t, at.Users[api.RoleRecruiter].ID, f.me(session(t, f.signInAs(flow, newAccount(org, email).claims(flow)))).User.Id)
}
