package app_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// apply sends the public apply form, signed out.
func (a *testApp) apply(path string, fields map[string]string, fileName string, cv []byte) *response {
	a.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(a.t, w.WriteField(k, v))
	}
	if fileName != "" {
		part, err := w.CreateFormFile("file", fileName)
		require.NoError(a.t, err)
		_, err = part.Write(cv)
		require.NoError(a.t, err)
	}
	require.NoError(a.t, w.Close())
	return a.request(http.MethodPost, path, "", &buf, "Content-Type", w.FormDataContentType())
}

func TestCareersPageAndApplying(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	var me api.Me
	a.request(http.MethodGet, "/v1/me", token, nil).JSON(&me)
	require.NotEmpty(t, me.Tenant.CareersSlug, "every company gets a careers page address")
	page := "/v1/public/careers/" + me.Tenant.CareersSlug

	company := a.createCompany(token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	newAd := func(body map[string]any) api.JobAd {
		return a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, body, http.StatusCreated)
	}
	publish := func(ad api.JobAd) {
		a.adRequest(http.MethodPatch, "/v1/job-ads/"+ad.Id, token, map[string]any{"version": ad.Version, "status": "published"}, http.StatusOK)
	}
	live := newAd(map[string]any{"title": "Senior Accountant", "description": "Join us.", "salaryText": "RM 8,000"})
	publish(live)
	newAd(map[string]any{"title": "Draft only", "description": "Not yet."})
	expiring := newAd(map[string]any{"title": "Closing soon", "description": "Hurry.", "closesOn": "2099-01-01"})
	publish(expiring)
	// Closing dates that pass after publishing hide the ad too.
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job_ad SET closes_on = current_date - 1 WHERE id = $1`, expiring.Id)
	require.NoError(t, err)

	res := a.request(http.MethodGet, page, "", nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var careers api.CareersPage
	res.JSON(&careers)
	require.Len(t, careers.Ads, 1, "only published ads before their closing date")
	assert.Equal(t, "Senior Accountant", careers.Ads[0].Title)
	assert.Equal(t, "RM 8,000", careers.Ads[0].SalaryText.MustGet())
	assert.NotContains(t, string(res.Body), job.Id, "no internal ids or fields")
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/public/careers/no-such-company", "", nil).Status)

	feed := a.request(http.MethodGet, page+"/jobs.xml", "", nil)
	require.Equal(t, http.StatusOK, feed.Status, "%s", feed.Body)
	assert.Contains(t, feed.Header.Get("Content-Type"), "application/xml")
	assert.Contains(t, string(feed.Body), "<title>Senior Accountant</title>")
	assert.Contains(t, string(feed.Body), "/careers/"+me.Tenant.CareersSlug+"/"+live.Id+"?via=indeed")
	assert.NotContains(t, string(feed.Body), "Draft only")

	t.Run("another company's ad is not found under this address", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		var otherMe api.Me
		a.request(http.MethodGet, "/v1/me", other, nil).JSON(&otherMe)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/public/careers/"+otherMe.Tenant.CareersSlug+"/ads/"+live.Id, "", nil).Status)
	})

	applyPath := page + "/ads/" + live.Id + "/applications"
	form := map[string]string{"firstName": "Nur", "lastName": "Aina", "email": "Nur.Aina@example.com", "phone": "012-345 6789", "consent": "yes"}

	t.Run("checks the form before doing anything", func(t *testing.T) {
		noConsent := map[string]string{"firstName": "Nur", "email": "nur@example.com"}
		assert.Equal(t, http.StatusBadRequest, a.apply(applyPath, noConsent, "cv.pdf", minimalPDF).Status)
		assert.Equal(t, http.StatusBadRequest, a.apply(applyPath, form, "", nil).Status, "a CV is required")
		assert.Equal(t, http.StatusNotFound, a.apply(page+"/ads/"+expiring.Id+"/applications", form, "cv.pdf", minimalPDF).Status)
	})

	res = a.apply(applyPath, form, "Nur CV.pdf", minimalPDF)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)

	list := a.request(http.MethodGet, "/v1/candidates?q=nur.aina", token, nil).Map()
	items := list["items"].([]any)
	require.Len(t, items, 1)
	candidateID := items[0].(map[string]any)["id"].(string)
	var cand api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+candidateID, token, nil).JSON(&cand)
	assert.Equal(t, api.CandidateSourceCareersPage, cand.Source.MustGet())
	assert.Equal(t, "+60123456789", cand.Phone.MustGet())
	docs := a.request(http.MethodGet, "/v1/candidates/"+candidateID+"/documents", token, nil).Map()["items"].([]any)
	require.Len(t, docs, 1, "the CV is attached")
	assert.Equal(t, "queued", docs[0].(map[string]any)["status"], "and is being read")
	apps := a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", token, nil).Map()["items"].([]any)
	require.Len(t, apps, 1)
	assert.Equal(t, "applied", apps[0].(map[string]any)["stage"])

	t.Run("applying again with a known email reuses the candidate, unchanged", func(t *testing.T) {
		again := map[string]string{"firstName": "Someone", "lastName": "Else", "email": "nur.aina@example.com", "consent": "yes"}
		require.Equal(t, http.StatusAccepted, a.apply(applyPath, again, "cv2.pdf", minimalPDF).Status)
		var same api.Candidate
		a.request(http.MethodGet, "/v1/candidates/"+candidateID, token, nil).JSON(&same)
		assert.Equal(t, "Nur", same.FirstName, "a public form never overwrites the profile")
		assert.Len(t, a.request(http.MethodGet, "/v1/candidates/"+candidateID+"/documents", token, nil).Map()["items"], 2)
		assert.Len(t, a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", token, nil).Map()["items"], 1, "still once on the job")
	})

	t.Run("robots filling the hidden field are answered but ignored", func(t *testing.T) {
		robot := map[string]string{"firstName": "Bot", "email": "bot@example.com", "consent": "yes", "website": "http://spam.example"}
		assert.Equal(t, http.StatusAccepted, a.apply(applyPath, robot, "cv.pdf", minimalPDF).Status)
		assert.Empty(t, a.request(http.MethodGet, "/v1/candidates?q=bot@example.com", token, nil).Map()["items"])
	})

	t.Run("a file that is not a CV rolls everything back", func(t *testing.T) {
		bad := map[string]string{"firstName": "Wei", "email": "wei@example.com", "consent": "yes"}
		assert.Equal(t, http.StatusUnsupportedMediaType, a.apply(applyPath, bad, "cv.exe", []byte("MZ not a cv")).Status)
		assert.Empty(t, a.request(http.MethodGet, "/v1/candidates?q=wei@example.com", token, nil).Map()["items"], "no candidate without their CV")
	})

	t.Run("the link's channel is recorded on the application and a new candidate", func(t *testing.T) {
		other := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk"})
		ad := a.adRequest(http.MethodPost, "/v1/jobs/"+other.Id+"/ads", token, map[string]any{"title": "Clerk", "description": "Join us."}, http.StatusCreated)
		publish(ad)
		viaLinkedIn := map[string]string{"firstName": "Li", "email": "li@example.com", "consent": "yes", "via": "linkedin"}
		require.Equal(t, http.StatusAccepted, a.apply(page+"/ads/"+ad.Id+"/applications", viaLinkedIn, "cv.pdf", minimalPDF).Status)
		apps := a.request(http.MethodGet, "/v1/jobs/"+other.Id+"/applications", token, nil).Map()["items"].([]any)
		require.Len(t, apps, 1)
		assert.Equal(t, "linkedin", apps[0].(map[string]any)["channel"])
		id := apps[0].(map[string]any)["candidateId"].(string)
		var li api.Candidate
		a.request(http.MethodGet, "/v1/candidates/"+id, token, nil).JSON(&li)
		assert.Equal(t, api.CandidateSourceLinkedin, li.Source.MustGet())

		// The first application above came without a link: the careers page.
		first := a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", token, nil).Map()["items"].([]any)
		assert.Equal(t, "careers_page", first[0].(map[string]any)["channel"])
	})

	t.Run("too many applications from one address are refused", func(t *testing.T) {
		var status int
		for i := range 6 {
			f := map[string]string{"firstName": "Burst", "email": "burst" + string(rune('a'+i)) + "@example.com", "consent": "yes"}
			if status = a.apply(applyPath, f, "cv.pdf", minimalPDF).Status; status == http.StatusTooManyRequests {
				break
			}
		}
		assert.Equal(t, http.StatusTooManyRequests, status)
	})
}
