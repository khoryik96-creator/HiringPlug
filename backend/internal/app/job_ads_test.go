package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) adRequest(method, path, token string, body any, want int) api.JobAd {
	a.t.Helper()
	res := a.request(method, path, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var ad api.JobAd
	res.JSON(&ad)
	return ad
}

func TestJobAds(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Accountant"})

	ad := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{
		"title": "  Senior   Accountant ", "location": "Kuala Lumpur", "workType": "full_time", "salaryText": "RM 8,000 - 10,000",
	}, http.StatusCreated)
	assert.Equal(t, api.JobAdStatusDraft, ad.Status)
	assert.Equal(t, "Senior Accountant", ad.Title)
	adPath := "/v1/job-ads/" + ad.Id

	t.Run("an ad needs its text before publishing", func(t *testing.T) {
		res := a.request(http.MethodPatch, adPath, token, map[string]any{"version": ad.Version, "status": "published"})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
		assert.Contains(t, string(res.Body), "JOB_AD_NOT_PUBLISHABLE")
		assert.Contains(t, string(res.Body), "Write the ad's text")
	})

	published := a.adRequest(http.MethodPatch, adPath, token, map[string]any{
		"version": ad.Version, "description": "We are hiring.\n- SAP FICO\n- Month-end close", "status": "published",
	}, http.StatusOK)
	assert.Equal(t, api.JobAdStatusPublished, published.Status)
	assert.True(t, published.PublishedAt.IsSpecified() && !published.PublishedAt.IsNull())
	assert.Equal(t, "We are hiring.\n- SAP FICO\n- Month-end close", published.Description.MustGet(), "line breaks kept")

	t.Run("a save based on an older version is a conflict", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPatch, adPath, token, map[string]any{"version": ad.Version, "title": "x"}).Status)
	})

	t.Run("expired closing dates cannot be published", func(t *testing.T) {
		other := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{
			"title": "Accountant", "description": "Text.", "closesOn": "2020-01-31",
		}, http.StatusCreated)
		res := a.request(http.MethodPatch, "/v1/job-ads/"+other.Id, token, map[string]any{"version": other.Version, "status": "published"})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
		assert.Contains(t, string(res.Body), "closing date has passed")
	})

	t.Run("viewers read ads but cannot change them; other companies cannot see them", func(t *testing.T) {
		viewer := tenant.Users[api.RoleViewer].Token
		list := a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/ads", viewer, nil).Map()
		assert.Len(t, list["items"], 2)
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPatch, adPath, viewer, map[string]any{"version": published.Version, "title": "x"}).Status)
		stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, adPath, stranger, nil).Status)
	})

	// On hold: the published ad stays up, but nothing new is published.
	res := a.request(http.MethodPatch, "/v1/jobs/"+job.Id, token, map[string]any{"version": job.Version, "status": "on_hold"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var onHold api.JobOrder
	res.JSON(&onHold)
	assert.Equal(t, api.JobAdStatusPublished, a.adRequest(http.MethodGet, adPath, token, nil, http.StatusOK).Status)
	draft := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{"title": "Clerk", "description": "Text."}, http.StatusCreated)
	res = a.request(http.MethodPatch, "/v1/job-ads/"+draft.Id, token, map[string]any{"version": draft.Version, "status": "published"})
	assert.Contains(t, string(res.Body), "this job is on hold")

	// Filled: its published ads come down in the same change.
	res = a.request(http.MethodPatch, "/v1/jobs/"+job.Id, token, map[string]any{"version": onHold.Version, "status": "filled"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, api.JobAdStatusClosed, a.adRequest(http.MethodGet, adPath, token, nil, http.StatusOK).Status)
	assert.Equal(t, api.JobAdStatusDraft, a.adRequest(http.MethodGet, "/v1/job-ads/"+draft.Id, token, nil, http.StatusOK).Status, "drafts stay drafts")
}
