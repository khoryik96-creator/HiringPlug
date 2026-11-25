package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A password is checked before the sign-in's transaction (it is slow); a
// password change or reset, or a deactivation, committed while the sign-in
// waits for the person's row signs nobody in: what was checked is checked
// again once the row is held.
func TestASignInUnderWayWhenThePasswordChangesSignsNobodyIn(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleAdmin]
	setPassword(t, recruiter.ID, testPassword)
	setPassword(t, admin.ID, testPassword)
	signIn := func(role api.Role) func() *response {
		return func() *response {
			res, _ := a.signIn(map[string]any{"email": emailOf(role, tenant.ID), "password": testPassword})
			return res
		}
	}
	refused := func(res *response) {
		t.Helper()
		require.Equal(t, http.StatusUnauthorized, res.Status, "%s", res.Body)
		assert.Equal(t, api.ErrorCodeSIGNINFAILED, res.Error().Error.Code)
	}

	// The password changed (its transaction held open) as they sign in with the old one.
	refused(meanwhile(t, `UPDATE app_user SET password_hash = 'another', tokens_valid_after = greatest(date_trunc('second', clock_timestamp()), tokens_valid_after) + interval '1 second'
		WHERE tenant_id = $1 AND id = $2`, []any{tenant.ID, recruiter.ID}, signIn(api.RoleRecruiter), nil))
	// Deactivated as they sign in.
	refused(meanwhile(t, `UPDATE app_user SET active = false WHERE tenant_id = $1 AND id = $2`, []any{tenant.ID, admin.ID}, signIn(api.RoleAdmin), nil))
}

// The two-step code finishes a sign-in only while the password it followed
// is still the person's: a challenge from before a password change or reset
// no longer works, even while the change is being made.
func TestATwoStepChallengeEndsWithAPasswordChange(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner]
	setPassword(t, owner.ID, testPassword)
	email := emailOf(api.RoleOwner, tenant.ID)
	var setup api.TwoStepSetup
	a.request(http.MethodPost, "/v1/me/two-step/setup", owner.Token, nil).JSON(&setup)
	res := a.request(http.MethodPost, "/v1/me/two-step/confirm", owner.Token, map[string]any{"code": nextCode(t, setup.Secret, 0)})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var codes api.TwoStepRecoveryCodes
	res.JSON(&codes)
	challenge := func() string {
		t.Helper()
		return challengeOf(t, a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": email, "password": testPassword}), api.ErrorCodeTWOSTEPREQUIRED)
	}
	finish := func(c, code string) *response {
		return a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": c, "code": code})
	}

	// Someone with the password (and soon a code) has a challenge; the
	// owner resets the password, and uses the new one.
	stolen := challenge()
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": email}).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	mine := challengeOf(t, a.request(http.MethodPost, "/v1/auth/links/"+linkIn(t, email), "", map[string]any{"password": "another-gecko-52"}), api.ErrorCodeTWOSTEPREQUIRED)
	res = finish(stolen, nextCode(t, setup.Secret, 1))
	require.Equal(t, http.StatusUnauthorized, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeSIGNINFAILED, res.Error().Error.Code)
	require.Equal(t, http.StatusOK, finish(mine, nextCode(t, setup.Secret, 1)).Status, "the challenge after the reset works")

	// A password change being made (held open) as the code is entered.
	stolen = func() string {
		t.Helper()
		return challengeOf(t, a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": email, "password": "another-gecko-52"}), api.ErrorCodeTWOSTEPREQUIRED)
	}()
	res = meanwhile(t, `UPDATE app_user SET password_hash = 'changed', tokens_valid_after = greatest(date_trunc('second', clock_timestamp()), tokens_valid_after) + interval '1 second'
		WHERE tenant_id = $1 AND id = $2`, []any{tenant.ID, owner.ID}, func() *response { return finish(stolen, codes.RecoveryCodes[0]) }, nil)
	require.Equal(t, http.StatusUnauthorized, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeSIGNINFAILED, res.Error().Error.Code)
}

// A reset link used twice at once sets one password: the second use waits
// for the first, and finds the link used.
func TestALinkIsUsedOnceEvenAtOnce(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	email := emailOf(api.RoleOwner, tenant.ID)
	setPassword(t, tenant.Users[api.RoleOwner].ID, testPassword)
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": email}).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	token := linkIn(t, email)
	res := meanwhile(t, `UPDATE user_link SET used_at = now() WHERE tenant_id = $1 AND kind = 'reset' AND used_at IS NULL`, []any{tenant.ID},
		func() *response {
			return a.request(http.MethodPost, "/v1/auth/links/"+token, "", map[string]any{"password": "a-long-new-password-1"})
		}, nil)
	require.Equal(t, http.StatusGone, res.Status, "%s", res.Body)
	res, _ = a.signIn(map[string]any{"email": email, "password": "a-long-new-password-1"})
	assert.Equal(t, http.StatusUnauthorized, res.Status, "the password was not set")
}
