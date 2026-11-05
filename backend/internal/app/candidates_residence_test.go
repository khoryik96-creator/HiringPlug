package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestCountryOfResidence(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token

	c := a.createCandidate(token, map[string]any{"firstName": "Aina", "residenceCountry": "SG"})
	assert.Equal(t, "SG", c.ResidenceCountry.MustGet())
	unknown := a.createCandidate(token, map[string]any{"firstName": "Bala"})
	assert.True(t, unknown.ResidenceCountry.IsNull(), "not known (NA)")

	res := a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": c.Version, "residenceCountry": "my"})
	assert.Equal(t, http.StatusBadRequest, res.Status, "a 2-letter code in capitals")
	res = a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": c.Version, "residenceCountry": "MY"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&c)
	assert.Equal(t, "MY", c.ResidenceCountry.MustGet())
}

func TestOneCandidatePerEmailNamesWhoHasIt(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	first := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.com"})

	res := a.request(http.MethodPost, "/v1/candidates", token, map[string]any{"firstName": "S", "email": "SITI@example.com"})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	e := res.Error()
	assert.Equal(t, api.ErrorCodeCANDIDATEDUPLICATEEMAIL, e.Error.Code)
	details := *e.Error.Details
	assert.Equal(t, first.Id, details["existingCandidateId"])
	assert.Equal(t, "Siti Aisyah", details["existingCandidateName"])
	assert.Equal(t, first.OwnerName.MustGet(), details["existingOwnerName"], "and who owns them")
}
