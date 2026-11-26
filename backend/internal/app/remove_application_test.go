package app_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestTakingACandidateOffAJob(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter, map[string]any{"title": "Auditor", "companyId": company.Id})
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aminah"})
	ali := a.createCandidate(recruiter, map[string]any{"firstName": "Ali", "lastName": "Hassan"})
	mistake := a.addToJob(recruiter, job.Id, siti.Id)
	kept := a.addToJob(recruiter, job.Id, ali.Id)

	remove := func(token, id string, version int) *response {
		return a.request(http.MethodDelete, "/v1/applications/"+id+"?version="+strconv.Itoa(version), token, nil)
	}
	assert.Equal(t, http.StatusForbidden, remove(tenant.Users[api.RoleViewer].Token, mistake.Id, mistake.Version).Status, "pipeline:update")
	assert.Equal(t, http.StatusConflict, remove(recruiter, mistake.Id, mistake.Version+1).Status, "changed since loaded")
	require.Equal(t, http.StatusNoContent, remove(recruiter, mistake.Id, mistake.Version).Status)
	var board api.ApplicationList
	a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter, nil).JSON(&board)
	require.Len(t, board.Items, 1, "off the job")
	assert.Equal(t, ali.Id, board.Items[0].CandidateId)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+siti.Id, recruiter, nil).Status, "the candidate stays")
	assert.Equal(t, http.StatusNotFound, remove(recruiter, mistake.Id, mistake.Version).Status)
	a.addToJob(recruiter, job.Id, siti.Id) // and can be put on it again

	// One with an interview is kept: withdrawn instead.
	a.interview(http.MethodPost, "/v1/applications/"+kept.Id+"/interviews", recruiter, map[string]any{
		"title": "First interview", "startsAt": time.Now().Add(72 * time.Hour).UTC().Truncate(time.Minute), "durationMinutes": 30, "mode": "phone",
		"interviewerIds": []string{tenant.Users[api.RoleRecruiter].ID},
	}, http.StatusCreated)
	a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter, nil).JSON(&board)
	version := 0
	for _, app := range board.Items {
		if app.Id == kept.Id {
			version = app.Version
		}
	}
	res := remove(recruiter, kept.Id, version)
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeAPPLICATIONNOTREMOVABLE, res.Error().Error.Code)
}
