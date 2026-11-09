package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestAFormattedCVCanBeDeletedButNeverTheCandidatesOwn(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	formatted, _ := a.formatCv(token, c.Id, map[string]any{}, http.StatusCreated)
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.txt", []byte("Siti Aisyah\nAuditor"))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var own api.Document
	res.JSON(&own)

	viewer := tenant.Users[api.RoleViewer].Token
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodDelete, "/v1/documents/"+formatted.DocumentId, viewer, nil).Status)
	res = a.request(http.MethodDelete, "/v1/documents/"+own.Id, token, nil)
	assert.Equal(t, api.ErrorCodeDOCUMENTNOTDELETABLE, res.Error().Error.Code, "the candidate's own CV is kept")
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/documents/"+formatted.DocumentId, other, nil).Status)

	require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/documents/"+formatted.DocumentId, token, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+formatted.DocumentId, token, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+formatted.DocumentId+"/file", token, nil).Status)
	var list api.DocumentList
	a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/documents", token, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, own.Id, list.Items[0].Id)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/documents/"+formatted.DocumentId, token, nil).Status, "already gone")
}
