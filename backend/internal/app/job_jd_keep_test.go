package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Uploaded job descriptions are kept with their job: listed, downloaded as
// uploaded, and deleted by those given jobs.documents:delete (admins).
func TestJobDescriptionsAreKeptWithTheJob(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter, api.RoleViewer)
	token, viewer, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleViewer].Token, tenant.Users[api.RoleAdmin].Token
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance"})

	list := func(tok string) []api.JdFile {
		t.Helper()
		res := a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/jd-files", tok, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var l api.JdFileList
		res.JSON(&l)
		return l.Items
	}

	var first, second api.JdFile
	a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "Finance Manager JD.txt", []byte(financeJD)).JSON(&first)
	a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "JD v2.pdf", minimalPDF).JSON(&second)

	t.Run("listed newest first, with their type and size", func(t *testing.T) {
		files := list(viewer)
		require.Len(t, files, 2)
		assert.Equal(t, second.Id, files[0].Id)
		assert.Equal(t, "application/pdf", files[0].ContentType)
		assert.Equal(t, len(minimalPDF), files[0].SizeBytes)
		assert.Equal(t, "Finance Manager JD.txt", files[1].FileName)
	})

	t.Run("downloaded as uploaded", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/jd-files/"+first.Id+"/file", viewer, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, financeJD, string(res.Body))
		assert.Contains(t, res.Header.Get("Content-Disposition"), "Finance Manager JD.txt")
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jd-files/"+first.Id+"/file", other, nil).Status, "only your company's")
	})

	t.Run("deleted only by admins; the file goes with it", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodDelete, "/v1/jd-files/"+first.Id, viewer, nil).Status)
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodDelete, "/v1/jd-files/"+first.Id, token, nil).Status, "a recruiter edits jobs but does not delete their files")
		require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/jd-files/"+first.Id, admin, nil).Status)
		files := list(token)
		require.Len(t, files, 1)
		assert.Equal(t, second.Id, files[0].Id)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jd-files/"+first.Id+"/file", token, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/jd-files/"+first.Id, admin, nil).Status)
		var n int
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'job.jd_deleted'`, job.Id).Scan(&n))
		assert.Equal(t, 1, n)
	})

	t.Run("another company's job has none to list", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/jd-files", other, nil).Status)
	})
}

// A job description dropped on the Add job form stays with the job made from it.
func TestTheJobDescriptionANewJobWasFilledInFromIsKept(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})

	var f api.JdFile
	a.upload("/v1/jd-files", token, "", "Finance Manager JD.txt", []byte(financeJD)).JSON(&f)
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance Manager", "jdFileId": f.Id})

	res := a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/jd-files", token, nil)
	var l api.JdFileList
	res.JSON(&l)
	require.Len(t, l.Items, 1)
	assert.Equal(t, f.Id, l.Items[0].Id)
	assert.Equal(t, job.Id, l.Items[0].JobId.MustGet())

	res = a.request(http.MethodPost, "/v1/jobs", token, map[string]any{"companyId": company.Id, "title": "Again", "jdFileId": f.Id})
	assert.Equal(t, http.StatusBadRequest, res.Status, "it already has its job: %s", res.Body)
}
