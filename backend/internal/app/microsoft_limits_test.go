package app_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Signing in with Microsoft has its own limits per address: a busy office
// behind one address signs in all morning, and neither way of signing in
// uses up the other's.
func TestMicrosoftSignInHasItsOwnLimits(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	setPassword(t, recruiter.ID, testPassword)
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))

	// 40 sign-ins from one address within 15 minutes: each starts, comes
	// back from Microsoft and finishes.
	for i := range 40 {
		flow, res := f.start(map[string]any{"company": slug, "email": sam.email})
		require.Equal(t, http.StatusOK, res.Status, "start %d: %s", i, res.Body)
		session(t, f.signInAs(flow, sam.claims(flow)))
	}
	// ...and password sign-ins from it still work: none of that counted.
	res, _ := f.a.signIn(map[string]any{"email": sam.email, "password": testPassword})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	// Wrong passwords use up the password limit, not Microsoft's.
	for range 40 {
		f.a.signIn(map[string]any{"email": "nobody." + tenant.ID + "@test.local", "password": "a-wrong-password-1"})
	}
	res, _ = f.a.signIn(map[string]any{"email": sam.email, "password": testPassword})
	assert.Equal(t, http.StatusTooManyRequests, res.Status, "the password limit is reached")
	flow, res := f.start(map[string]any{"company": slug})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	session(t, f.signInAs(flow, sam.claims(flow)))
}

// The email typed when starting a Microsoft sign-in is only a hint to
// Microsoft: starting many (from anywhere) never uses up password sign-in's
// limit for that email.
func TestMicrosoftStartsUseNoPasswordLimitOfTheEmail(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{
		"HP_PUBLIC_URL": "http://app.test", "HP_MAIL_MICROSOFT_CLIENT_ID": "server-client", "HP_MAIL_MICROSOFT_CLIENT_SECRET": "server-secret",
		"HP_CLIENT_IP_HEADER": "X-Real-IP",
	})
	f := &msTest{t: t, a: a}
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	setPassword(t, tenant.Users[api.RoleRecruiter].ID, testPassword)
	f.enable(tenant.Users[api.RoleOwner].Token, newGUID(), false)
	slug := f.me(tenant.Users[api.RoleOwner].Token).Tenant.CareersSlug
	email := emailOf(api.RoleRecruiter, tenant.ID)
	for i := range 210 { // more than password sign-in allows per email in 15 minutes (200), from 3 addresses
		res := a.request(http.MethodPost, "/v1/auth/microsoft/start", "",
			map[string]any{"bindingHash": bindingHash(randomBinding()), "company": slug, "email": email}, "X-Real-IP", fmt.Sprintf("192.0.2.%d", 1+i%3))
		require.Equal(t, http.StatusOK, res.Status, "start %d: %s", i, res.Body)
	}
	res := a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": email, "password": testPassword}, "X-Real-IP", "192.0.2.9")
	assert.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
}
