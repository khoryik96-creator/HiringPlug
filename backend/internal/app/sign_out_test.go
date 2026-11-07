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

func TestSigningOutEndsThatSessionAtOnce(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	other, err := testutil.Verifier().Mint(tenant.ID, user.ID, time.Hour) // the same person in another browser
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", user.Token, nil).Status)
	assert.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/auth/sign-out", user.Token, nil).Status)
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", user.Token, nil).Status, "a copied token stops working too")
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", other, nil).Status, "their other sessions go on")
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodPost, "/v1/auth/sign-out", user.Token, nil).Status)
}
