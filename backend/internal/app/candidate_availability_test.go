package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestACandidatesStatusColoursTheirPage(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token

	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "availability": "passive"})
	assert.Equal(t, api.CandidateAvailabilityPassive, c.Availability.MustGet())
	none := a.createCandidate(token, map[string]any{"firstName": "Ali"})
	assert.True(t, none.Availability.IsNull(), "not set until someone sets it")

	res := a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": c.Version, "availability": "not_looking"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var updated api.Candidate
	res.JSON(&updated)
	assert.Equal(t, api.CandidateAvailabilityNotLooking, updated.Availability.MustGet())
	assert.Equal(t, "Siti", updated.FirstName, "nothing else changes")

	res = a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": updated.Version, "availability": "busy"})
	assert.Equal(t, http.StatusBadRequest, res.Status, "only the four statuses")

	res = a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": updated.Version, "availability": nil})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&updated)
	assert.True(t, updated.Availability.IsNull(), "and it can be cleared")

	t.Run("a merge keeps the kept candidate's status, or takes the duplicate's", func(t *testing.T) {
		keep := a.createCandidate(token, map[string]any{"firstName": "Keep"})
		dup := a.createCandidate(token, map[string]any{"firstName": "Dup", "availability": "looking"})
		res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var merged api.Candidate
		res.JSON(&merged)
		assert.Equal(t, api.CandidateAvailabilityLooking, merged.Availability.MustGet())
	})
}
