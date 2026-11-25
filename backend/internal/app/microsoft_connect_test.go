package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// startRefused checks starting to connect was refused for field.
func startRefused(t *testing.T, res *response, field string) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Contains(t, string(res.Body), `"field":"`+field+`"`)
}

// A session left open on someone's computer does not connect a Microsoft
// account to them: connecting asks for their password, and their two-step
// code (or a recovery code) when they use it, limited like changing the
// password.
func TestConnectingMicrosoftAsksWhoYouAre(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	owner, admin := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin]
	setPassword(t, owner.ID, testPassword)
	setPassword(t, admin.ID, testPassword)
	org := newGUID()
	f.enable(owner.Token, org, false)
	boss := newAccount(org, "boss."+tenant.ID+"@contoso.test")

	_, res := f.startConnecting(owner.Token)
	startRefused(t, res, "password")
	_, res = f.startConnecting(owner.Token, map[string]any{"password": "not-the-password-1"})
	startRefused(t, res, "password")
	_, res = f.startConnecting(owner.Token, map[string]any{"password": testPassword})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	// With two-step sign-in on: the app's code, or a recovery code, too.
	var setup api.TwoStepSetup
	f.a.request(http.MethodPost, "/v1/me/two-step/setup", owner.Token, nil).JSON(&setup)
	var codes api.TwoStepRecoveryCodes
	res = f.a.request(http.MethodPost, "/v1/me/two-step/confirm", owner.Token, map[string]any{"code": nextCode(t, setup.Secret, 0)})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&codes)
	_, res = f.startConnecting(owner.Token, map[string]any{"password": testPassword})
	startRefused(t, res, "code")
	_, res = f.startConnecting(owner.Token, map[string]any{"password": testPassword, "code": "000000"})
	startRefused(t, res, "code")
	_, res = f.startConnecting(owner.Token, map[string]any{"password": "not-the-password-1", "code": nextCode(t, setup.Secret, 1)})
	startRefused(t, res, "password")
	_, res = f.startConnecting(owner.Token, map[string]any{"password": testPassword, "code": codes.RecoveryCodes[0]})
	require.Equal(t, http.StatusOK, res.Status, "a recovery code: %s", res.Body)
	res = f.connect(owner.Token, boss.claims, map[string]any{"password": testPassword, "code": nextCode(t, setup.Secret, 1)})
	require.Equal(t, http.StatusNoContent, res.Status, "%s", res.Body)
	assert.True(t, f.me(owner.Token).MicrosoftLinked)

	// Guessing is limited like changing the password (per person).
	for range 10 {
		_, res = f.startConnecting(admin.Token, map[string]any{"password": "a-wrong-guess-1"})
		startRefused(t, res, "password")
	}
	_, res = f.startConnecting(admin.Token, map[string]any{"password": testPassword})
	assert.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
}

// Connecting a Microsoft account is for a person signed in, never an API key.
func TestAPIKeysDoNotConnectMicrosoft(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	f.enable(tenant.Users[api.RoleOwner].Token, newGUID(), false)
	var key api.ApiKeyCreated
	res := f.a.request(http.MethodPost, "/v1/api-keys", tenant.Users[api.RoleAdmin].Token, map[string]any{"name": "Sync", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&key)

	_, res = f.startConnecting(key.Key)
	assert.Equal(t, http.StatusForbidden, res.Status, "%s", res.Body)
	res = f.finishConnecting(key.Key, "t"+randomBinding(), randomBinding())
	require.Equal(t, http.StatusForbidden, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeFORBIDDEN, res.Error().Error.Code)
}

// Connecting asks for the two-step code as signing in does: a code works
// once (a code seen, or used to sign in, does not connect), and codes tried
// here count against the person's limit for codes, so guessing here does
// not add to guessing at sign-in.
func TestConnectingTakesTwoStepCodesLikeSigningIn(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	f.enable(tenant.Users[api.RoleOwner].Token, newGUID(), false)
	withTwoStep := func(role api.Role) (testutil.User, string) {
		t.Helper()
		u := tenant.Users[role]
		setPassword(t, u.ID, testPassword)
		var setup api.TwoStepSetup
		f.a.request(http.MethodPost, "/v1/me/two-step/setup", u.Token, nil).JSON(&setup)
		res := f.a.request(http.MethodPost, "/v1/me/two-step/confirm", u.Token, map[string]any{"code": nextCode(t, setup.Secret, 0)})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		return u, setup.Secret
	}

	owner, secret := withTwoStep(api.RoleOwner)
	code := nextCode(t, secret, 1)
	_, res := f.startConnecting(owner.Token, map[string]any{"password": testPassword, "code": code})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	_, res = f.startConnecting(owner.Token, map[string]any{"password": testPassword, "code": code})
	startRefused(t, res, "code")

	admin, secret := withTwoStep(api.RoleAdmin)
	for range 9 { // with the code that confirmed the app: the limit for codes (10)
		_, res = f.startConnecting(admin.Token, map[string]any{"password": testPassword, "code": "000000"})
		startRefused(t, res, "code")
	}
	challenge := challengeOf(t, f.a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": emailOf(api.RoleAdmin, tenant.ID), "password": testPassword}), api.ErrorCodeTWOSTEPREQUIRED)
	res = f.a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge, "code": nextCode(t, secret, 1)})
	assert.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
}
