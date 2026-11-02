package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestSimilarCandidatesFlagsPossibleDuplicates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	siti := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "phone": "012-345 6789"})
	a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aishah"})                        // spelled differently
	a.createCandidate(token, map[string]any{"firstName": "S.", "lastName": "Rahman", "phone": "+60123456789"}) // same phone
	a.createCandidate(token, map[string]any{"firstName": "Wei", "lastName": "Tan"})                            // someone else

	var list api.SimilarCandidateList
	res := a.request(http.MethodGet, "/v1/candidates/"+siti.Id+"/similar", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	require.Len(t, list.Items, 2)
	reasons := map[string][]api.SimilarCandidateReasons{}
	for _, s := range list.Items {
		reasons[s.Candidate.FullName] = s.Reasons
	}
	assert.Equal(t, []api.SimilarCandidateReasons{api.SimilarCandidateReasonsSimilarName}, reasons["Siti Aishah"])
	assert.Equal(t, []api.SimilarCandidateReasons{api.SimilarCandidateReasonsSamePhone}, reasons["S. Rahman"])

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	res = a.request(http.MethodGet, "/v1/candidates/"+siti.Id+"/similar", other.Users[api.RoleOwner].Token, nil)
	assert.Equal(t, http.StatusNotFound, res.Status)
}
