package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// nextCode is the app's code a step ahead (a code is accepted once per step;
// a step either side of now is allowed).
func nextCode(t *testing.T, secret string, ahead int64) string {
	t.Helper()
	code, err := identity.TOTPCode(secret, identity.TOTPStep(time.Now())+ahead)
	require.NoError(t, err)
	return code
}

func challengeOf(t *testing.T, res *response, want api.ErrorCode) string {
	t.Helper()
	require.Equal(t, http.StatusUnauthorized, res.Status, "%s", res.Body)
	e := res.Error()
	require.Equal(t, want, e.Error.Code)
	require.NotNil(t, e.Error.Details)
	c, _ := (*e.Error.Details)["challenge"].(string)
	require.NotEmpty(t, c)
	return c
}

func TestTwoStepSignIn(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleAdmin)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	setPassword(t, owner.ID, testPassword)
	setPassword(t, recruiter.ID, testPassword)
	ownerEmail, recruiterEmail := emailOf(api.RoleOwner, tenant.ID), emailOf(api.RoleRecruiter, tenant.ID)

	// The owner adds an authenticator app.
	var setup api.TwoStepSetup
	res := a.request(http.MethodPost, "/v1/me/two-step/setup", owner.Token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&setup)
	assert.Contains(t, setup.Uri, "otpauth://totp/")
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/me/two-step/confirm", owner.Token, map[string]any{"code": "000000"}).Status)
	var codes api.TwoStepRecoveryCodes
	res = a.request(http.MethodPost, "/v1/me/two-step/confirm", owner.Token, map[string]any{"code": nextCode(t, setup.Secret, 0)})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&codes)
	require.Len(t, codes.RecoveryCodes, 10)

	// Signing in now takes the password, then a code: each code works once.
	challenge := challengeOf(t, a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": ownerEmail, "password": testPassword}), api.ErrorCodeTWOSTEPREQUIRED)
	wrong := a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge, "code": "123456"})
	assert.Equal(t, api.ErrorCodeSIGNINFAILED, wrong.Error().Error.Code)
	code := nextCode(t, setup.Secret, 1)
	res = a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge, "code": code})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var session api.Session
	res.JSON(&session)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", session.Token, nil).Status)
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge, "code": code}).Status, "used already")
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge + "x", "code": nextCode(t, setup.Secret, 1)}).Status, "a challenge that was changed")

	// A recovery code, once.
	res = a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge, "code": codes.RecoveryCodes[0]})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": challenge, "code": codes.RecoveryCodes[0]}).Status)
	var status api.TwoStepStatus
	a.request(http.MethodGet, "/v1/me/two-step", owner.Token, nil).JSON(&status)
	assert.True(t, status.Enabled)
	assert.Equal(t, 9, status.RecoveryCodesLeft)

	// A password reset link does not skip the code.
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": ownerEmail}).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	resetChallenge := challengeOf(t, a.request(http.MethodPost, "/v1/auth/links/"+linkIn(t, ownerEmail), "", map[string]any{"password": "another-gecko-52"}), api.ErrorCodeTWOSTEPREQUIRED)
	time.Sleep(1100 * time.Millisecond) // the reset ended the owner's sessions at the next whole second
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", owner.Token, nil).Status, "the reset signed out the old session")
	res = a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": resetChallenge, "code": codes.RecoveryCodes[1]})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var ownerSession api.Session
	res.JSON(&ownerSession)
	owner.Token = ownerSession.Token
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", owner.Token, nil).Status, "a session minted after the reset works")

	// Requiring it of everyone signs out the recruiter, who sets it up at their next sign-in.
	_, recruiterToken := a.signIn(map[string]any{"email": recruiterEmail, "password": testPassword})
	require.NotEmpty(t, recruiterToken)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/agency/two-step", recruiter.Token, map[string]any{"required": true}).Status)
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/agency/two-step", owner.Token, map[string]any{"required": true}).Status)
	time.Sleep(1100 * time.Millisecond) // sessions end at the next whole second
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", recruiterToken, nil).Status, "signed out")
	setupChallenge := challengeOf(t, a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": recruiterEmail, "password": testPassword}), api.ErrorCodeTWOSTEPSETUPREQUIRED)
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodPost, "/v1/auth/two-step", "", map[string]any{"challenge": setupChallenge, "code": "123456"}).Status, "set up first")
	var recruiterSetup api.TwoStepSetup
	res = a.request(http.MethodPost, "/v1/auth/two-step/setup", "", map[string]any{"challenge": setupChallenge})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&recruiterSetup)
	var enabled api.TwoStepEnabled
	res = a.request(http.MethodPost, "/v1/auth/two-step/setup/confirm", "", map[string]any{"challenge": setupChallenge, "code": nextCode(t, recruiterSetup.Secret, 0)})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&enabled)
	assert.Len(t, enabled.RecoveryCodes, 10)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", enabled.Session.Token, nil).Status)
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodPost, "/v1/auth/two-step/setup", "", map[string]any{"challenge": setupChallenge}).Status,
		"a setup challenge cannot replace an app once one is on")
	disable := a.request(http.MethodPost, "/v1/me/two-step/disable", enabled.Session.Token, map[string]any{"password": testPassword})
	assert.Equal(t, api.ErrorCodeTEAMCHANGENOTALLOWED, disable.Error().Error.Code, "not while the company requires it")

	// The team list shows who has it; an admin resets a lost phone (not an owner's).
	var team api.TeamList
	a.request(http.MethodGet, "/v1/team", owner.Token, nil).JSON(&team)
	for _, m := range team.Items {
		assert.Equal(t, m.Id == owner.ID || m.Id == recruiter.ID, m.TwoStep, m.Name) // before the admin sets theirs up
	}
	// The admin was signed out too; they set it up at sign-in.
	setPassword(t, tenant.Users[api.RoleAdmin].ID, testPassword)
	adminChallenge := challengeOf(t, a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": emailOf(api.RoleAdmin, tenant.ID), "password": testPassword}), api.ErrorCodeTWOSTEPSETUPREQUIRED)
	var adminSetup api.TwoStepSetup
	a.request(http.MethodPost, "/v1/auth/two-step/setup", "", map[string]any{"challenge": adminChallenge}).JSON(&adminSetup)
	var adminEnabled api.TwoStepEnabled
	a.request(http.MethodPost, "/v1/auth/two-step/setup/confirm", "", map[string]any{"challenge": adminChallenge, "code": nextCode(t, adminSetup.Secret, 0)}).JSON(&adminEnabled)
	admin := adminEnabled.Session.Token
	require.NotEmpty(t, admin)
	assert.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodDelete, "/v1/team/"+owner.ID+"/two-step", admin, nil).Status)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/team/"+recruiter.ID+"/two-step", admin, nil).Status)
	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", enabled.Session.Token, nil).Status, "a reset signs them out")
	challengeOf(t, a.request(http.MethodPost, "/v1/auth/sign-in", "", map[string]any{"email": recruiterEmail, "password": testPassword}), api.ErrorCodeTWOSTEPSETUPREQUIRED)
}
