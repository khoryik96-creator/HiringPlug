package app_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestJobsHaveACountryAndAnIntakeDate(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Contoh"})

	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk", "country": "SG"})
	assert.Equal(t, time.Now().UTC().Format(time.DateOnly), job.IntakeDate.Format(time.DateOnly), "today unless chosen")
	assert.Equal(t, "SG", job.Country.MustGet())

	job = a.createJob(token, map[string]any{"companyId": company.Id, "title": "Analyst", "intakeDate": "2026-10-01"})
	assert.Equal(t, "2026-10-01", job.IntakeDate.Format(time.DateOnly))
	res := a.request(http.MethodPatch, "/v1/jobs/"+job.Id, token, map[string]any{"version": job.Version, "intakeDate": "2026-10-15", "country": "MY"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&job)
	assert.Equal(t, "2026-10-15", job.IntakeDate.Format(time.DateOnly))
	res = a.request(http.MethodPatch, "/v1/jobs/"+job.Id, token, map[string]any{"version": job.Version, "country": "my"})
	assert.Equal(t, http.StatusBadRequest, res.Status, "a 2-letter code in capitals")

	ad := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{"title": "Analyst"}, http.StatusCreated)
	assert.Equal(t, "MY", ad.Country.MustGet(), "an ad takes its job's country")
}

func TestBlindJobAds(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Logistik Sdn Bhd", "website": "https://majulogistik.my"})
	contact := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "contactId": contact.Id, "title": "Ops Manager"})

	ad := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{
		"title": "Ops Manager at Maju Logistik", "clientDescription": "a regional logistics company",
		"description": "Maju Logistik is growing.\n- Run the Shah Alam hub\nSend your CV to Aisyah Rahman, hr@majulogistik.my.",
	}, http.StatusCreated)
	assert.True(t, ad.PublishBlind, "new ads go out blind")
	assert.Equal(t, "Ops Manager at our client", ad.BlindTitle)
	assert.Equal(t, "A regional logistics company is growing.\n- Run the Shah Alam hub\nSend your CV to the hiring manager, [email removed].", ad.BlindDescription.MustGet())
	assert.False(t, ad.BlindEdited)

	patch := func(body map[string]any) *response {
		return a.request(http.MethodPatch, "/v1/job-ads/"+ad.Id, token, body)
	}
	t.Run("a person's own blind text may not name the client", func(t *testing.T) {
		res := patch(map[string]any{"version": ad.Version, "blindDescription": "Join Maju Logistik!"})
		assert.Equal(t, []string{"blindDescription"}, fieldNames(t, res.Error()))
	})

	res := patch(map[string]any{"version": ad.Version, "blindDescription": "Join a fast-growing logistics firm."})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&ad)
	assert.True(t, ad.BlindEdited)
	assert.False(t, ad.BlindOutdated)
	assert.Equal(t, "Ops Manager at our client", ad.BlindTitle, "the title stays as made")

	res = patch(map[string]any{"version": ad.Version, "description": "Maju Logistik is hiring fast."})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&ad)
	assert.Equal(t, "Join a fast-growing logistics firm.", ad.BlindDescription.MustGet(), "a person's text is kept")
	assert.True(t, ad.BlindOutdated, "but marked as written for an older ad")

	t.Run("published blind on the careers page", func(t *testing.T) {
		res := patch(map[string]any{"version": ad.Version, "status": "published"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&ad)
		var me api.Me
		a.request(http.MethodGet, "/v1/me", token, nil).JSON(&me)
		var public api.PublicJobAd
		a.request(http.MethodGet, "/v1/public/careers/"+me.Tenant.CareersSlug+"/ads/"+ad.Id, "", nil).JSON(&public)
		assert.Equal(t, "Ops Manager at our client", public.Title)
		assert.Equal(t, "Join a fast-growing logistics firm.", public.Description)
		feed := a.request(http.MethodGet, "/v1/public/careers/"+me.Tenant.CareersSlug+"/jobs.xml", "", nil)
		assert.NotContains(t, string(feed.Body), "Maju")
	})

	t.Run("remade from the ad on request", func(t *testing.T) {
		res := patch(map[string]any{"version": ad.Version, "remakeBlind": true})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&ad)
		assert.False(t, ad.BlindEdited)
		assert.Equal(t, "A regional logistics company is hiring fast.", ad.BlindDescription.MustGet())
	})

	t.Run("exported as Word and PDF", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/job-ads/"+ad.Id+"/export?version=blind&format=docx", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Contains(t, res.Header.Get("Content-Disposition"), `filename="Ops Manager at our client (blind).docx"`)
		zr, err := zip.NewReader(bytes.NewReader(res.Body), int64(len(res.Body)))
		require.NoError(t, err)
		var all bytes.Buffer
		for _, f := range zr.File {
			r, _ := f.Open()
			_, _ = all.ReadFrom(r)
		}
		assert.Contains(t, all.String(), "A regional logistics company is hiring fast.")
		assert.NotContains(t, all.String(), "Maju")

		res = a.request(http.MethodGet, "/v1/job-ads/"+ad.Id+"/export?version=original&format=pdf", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.True(t, bytes.HasPrefix(res.Body, []byte("%PDF-")))
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/job-ads/"+ad.Id+"/export?version=blind&format=pdf", other, nil).Status)
	})
}
