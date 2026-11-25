package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (f *msTest) methodsOf(ownerToken, userID string) []api.SignInMethod {
	f.t.Helper()
	var team api.TeamList
	f.a.request(http.MethodGet, "/v1/team", ownerToken, nil).JSON(&team)
	for _, m := range team.Items {
		if m.Id == userID {
			return m.SignInMethods
		}
	}
	f.t.Fatalf("%s is not on the team", userID)
	return nil
}

func (f *msTest) unlink(token, userID string) *response {
	return f.a.request(http.MethodDelete, "/v1/team/"+userID+"/microsoft", token, nil)
}

func status(a *testApp, token string) int {
	return a.request(http.MethodGet, "/v1/me", token, nil).Status
}

// Owners (and admins, for everyone but owners) unlink a member's Microsoft
// account on the Team page: their Microsoft sessions end, their password
// ones go on, and they can link one again.
func TestUnlinkingMicrosoft(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	setPassword(t, recruiter.ID, testPassword)
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	flow, _ := f.start(map[string]any{"company": slug})
	samMicrosoft := session(t, f.signInAs(flow, sam.claims(flow)))
	_, samPassword := f.a.signIn(map[string]any{"email": sam.email, "password": testPassword})
	require.NotEmpty(t, samPassword)
	require.Equal(t, http.StatusNoContent, f.connect(owner.Token, newAccount(org, "boss@contoso.test").claims).Status)
	assert.ElementsMatch(t, []api.SignInMethod{api.SignInMethodPassword, api.SignInMethodMicrosoft}, f.methodsOf(owner.Token, recruiter.ID))

	// Who may: not a recruiter; not an admin, for an owner.
	assert.Equal(t, http.StatusForbidden, f.unlink(recruiter.Token, recruiter.ID).Status)
	res := f.unlink(admin.Token, owner.ID)
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeTEAMCHANGENOTALLOWED, res.Error().Error.Code)

	res = f.unlink(admin.Token, recruiter.ID)
	require.Equal(t, http.StatusNoContent, res.Status, "%s", res.Body)
	assert.Equal(t, []string{`{"organisationId": "` + org + `"}`}, audited(t, tenant.ID, "user.microsoft_unlinked"))
	assert.Equal(t, http.StatusUnauthorized, status(f.a, samMicrosoft), "their Microsoft session ended")
	assert.Equal(t, http.StatusOK, status(f.a, samPassword), "their password session goes on")
	assert.Equal(t, []api.SignInMethod{api.SignInMethodPassword}, f.methodsOf(owner.Token, recruiter.ID))
	assert.Equal(t, http.StatusNotFound, f.unlink(admin.Token, recruiter.ID).Status, "nothing linked")

	// They link one again by signing in (here, at once: the same second).
	flow, _ = f.start(map[string]any{"company": slug})
	again := session(t, f.signInAs(flow, sam.claims(flow)))
	assert.Equal(t, http.StatusOK, status(f.a, again))
	assert.Len(t, audited(t, tenant.ID, "user.microsoft_linked"), 3)

	// Owners unlink owners (their own too); they connect again from Your account.
	require.Equal(t, http.StatusNoContent, f.unlink(owner.Token, owner.ID).Status)
	assert.False(t, f.me(owner.Token).MicrosoftLinked, "a password session goes on")
	require.Equal(t, http.StatusNoContent, f.connect(owner.Token, newAccount(org, "boss@contoso.test").claims).Status)
}

// Changing the organisation removes the links to the old one's accounts and
// ends Microsoft sessions; everyone links again with an account of the new
// one. Turning Microsoft off ends Microsoft sessions too.
func TestChangingTheMicrosoftOrganisation(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	email := emailOf(api.RoleRecruiter, tenant.ID)
	oldOrg, newOrg := newGUID(), newGUID()
	f.enable(owner.Token, oldOrg, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	flow, _ := f.start(map[string]any{"company": slug})
	before := session(t, f.signInAs(flow, newAccount(oldOrg, email).claims(flow)))

	f.enable(owner.Token, newOrg, false)
	changes := audited(t, tenant.ID, "agency.microsoft_sign_in_changed")
	assert.Contains(t, changes[len(changes)-1], `"linksRemoved": 1`)
	assert.Equal(t, http.StatusUnauthorized, status(f.a, before), "a session of the old organisation's account")
	assert.Equal(t, http.StatusOK, status(f.a, owner.Token), "password sessions go on")
	assert.Empty(t, f.methodsOf(owner.Token, recruiter.ID))

	flow, _ = f.start(map[string]any{"company": slug})
	msRefused(t, f.signInAs(flow, newAccount(oldOrg, email).claims(flow)), "other_organisation")
	flow, _ = f.start(map[string]any{"company": slug})
	after := session(t, f.signInAs(flow, newAccount(newOrg, email).claims(flow)))
	assert.Equal(t, recruiter.ID, f.me(after).User.Id, "linked again, to the new organisation's account")

	// A link to another organisation left over does not keep them from linking.
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE user_microsoft SET ms_tenant_id = $1 WHERE tenant_id = $2 AND user_id = $3`, newGUID(), tenant.ID, recruiter.ID)
	require.NoError(t, err)
	sam := newAccount(newOrg, email)
	flow, _ = f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow)))

	// Turned off: Microsoft sessions stop, and stay stopped when it is turned on again.
	flow, _ = f.start(map[string]any{"company": slug})
	last := session(t, f.signInAs(flow, sam.claims(flow)))
	res := f.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", owner.Token, map[string]any{"enabled": false, "organisationId": newOrg, "required": false})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusUnauthorized, status(f.a, last))
	f.enable(owner.Token, newOrg, false)
	assert.Equal(t, http.StatusUnauthorized, status(f.a, last))
	flow, _ = f.start(map[string]any{"company": slug})
	assert.Equal(t, http.StatusOK, status(f.a, session(t, f.signInAs(flow, sam.claims(flow)))), "a new sign-in works")
}

// A session signed in with Microsoft works only while the company signs in
// with Microsoft.
func TestMicrosoftSessionsNeedMicrosoftOn(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	org := newGUID()
	f.enable(tenant.Users[api.RoleOwner].Token, org, false)
	flow, _ := f.start(map[string]any{"company": f.me(tenant.Users[api.RoleOwner].Token).Tenant.CareersSlug})
	token := session(t, f.signInAs(flow, newAccount(org, emailOf(api.RoleRecruiter, tenant.ID)).claims(flow)))
	// Off by any means (here the database), the session is refused.
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE tenant SET microsoft_sign_in = false WHERE id = $1`, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, status(f.a, token))
}
