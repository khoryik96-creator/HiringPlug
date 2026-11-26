package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestASignInIsRenewedWhileWorking(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	owner := tenant.Users[api.RoleOwner]
	setPassword(t, owner.ID, testPassword)
	res, first := a.signIn(map[string]any{"email": emailOf(api.RoleOwner, tenant.ID), "password": testPassword})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var signedIn api.Session
	res.JSON(&signedIn)

	time.Sleep(1100 * time.Millisecond) // a later second: a later expiry
	res = a.request(http.MethodPost, "/v1/auth/renew", first, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var renewed api.Session
	res.JSON(&renewed)
	assert.True(t, renewed.ExpiresAt.After(signedIn.ExpiresAt), "12 hours from now, later than before")
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", renewed.Token, nil).Status)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", first, nil).Status, "the same session: other tabs keep working")

	// Signing out with the renewed token ends the session: every token of it.
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/auth/sign-out", renewed.Token, nil).Status)
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", first, nil).Status, "the first token ends with it")
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodPost, "/v1/auth/renew", first, nil).Status, "never renewed after signing out")

	// An API key has no sign-in to renew.
	res = a.request(http.MethodPost, "/v1/api-keys", tenant.Users[api.RoleAdmin].Token, map[string]any{"name": "Sync", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var key api.ApiKeyCreated
	res.JSON(&key)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/auth/renew", key.Key, nil).Status)
}
