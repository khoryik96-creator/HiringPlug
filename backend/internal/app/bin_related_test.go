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

// A binned candidate's (or binned company's) interviews and offers leave the
// lists with them, instead of staying as rows with no candidate or job, and
// come back when they are restored.
func TestBinnedRecordsTakeTheirInterviewsAndOffersWithThem(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin, recruiter := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token

	scheduled := func(hours int) (api.Interview, api.Offer, api.Company) {
		c := a.createCandidate(recruiter, map[string]any{"firstName": "Siti"})
		company := a.createCompany(recruiter, map[string]any{"name": "Contoh " + time.Now().Format("150405.000000")})
		job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Accountant"})
		app := a.addToJob(recruiter, job.Id, c.Id)
		iv := a.interview(http.MethodPost, "/v1/applications/"+app.Id+"/interviews", recruiter, map[string]any{
			"title": "First interview", "startsAt": time.Now().Add(time.Duration(hours) * time.Hour), "durationMinutes": 45, "mode": "video",
			"interviewerIds": []string{tenant.Users[api.RoleRecruiter].ID},
		}, http.StatusCreated)
		of := a.offer(http.MethodPost, "/v1/applications/"+app.Id+"/offers", recruiter, map[string]any{"terms": "Medical"}, http.StatusCreated)
		return iv, of, company
	}
	listed := func() (interviews, offers []string) {
		var ivs api.InterviewList
		a.request(http.MethodGet, "/v1/interviews", recruiter, nil).JSON(&ivs) // checked against the contract: no blank rows
		for _, i := range ivs.Items {
			interviews = append(interviews, i.Id)
		}
		var ofs api.OfferList
		a.request(http.MethodGet, "/v1/offers", recruiter, nil).JSON(&ofs)
		for _, o := range ofs.Items {
			offers = append(offers, o.Id)
		}
		return interviews, offers
	}

	iv, of, _ := scheduled(72)
	other, otherOffer, company := scheduled(96)
	res := a.bin(admin, map[string]any{"kind": "candidate", "id": iv.CandidateId})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var entry api.BinEntry
	res.JSON(&entry)
	interviews, offers := listed()
	assert.NotContains(t, interviews, iv.Id, "a binned candidate's interview leaves the list")
	assert.NotContains(t, offers, of.Id, "and their offer")
	assert.Contains(t, interviews, other.Id, "other candidates' stay")
	assert.Contains(t, offers, otherOffer.Id)

	require.Equal(t, http.StatusCreated, a.bin(admin, map[string]any{"kind": "company", "id": company.Id}).Status)
	interviews, offers = listed()
	assert.NotContains(t, interviews, other.Id, "a binned company's jobs take their interviews")
	assert.NotContains(t, offers, otherOffer.Id)

	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/bin/"+entry.Id+"/restore", admin, nil).Status)
	interviews, offers = listed()
	assert.Contains(t, interviews, iv.Id, "restored, they are back")
	assert.Contains(t, offers, of.Id)
}
