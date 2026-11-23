package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A candidate deleted for good leaves the client's links with them: the
// client never sees someone who is gone.
func TestAClientLinkForgetsACandidateDeletedForGood(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	sj := a.shortlistJob(recruiter, tenant.ID, map[string]map[string]any{"Siti": {}, "Wei": {}})
	_, token := a.makeShortlist(recruiter, sj.job.Id, map[string]any{"candidates": []any{pick(sj, "Siti"), pick(sj, "Wei")}}, http.StatusCreated)

	require.Equal(t, http.StatusCreated, a.bin(admin, map[string]any{"kind": "candidate", "id": sj.candidates["Siti"].Id}).Status)
	purgeNow(t, a, tenant.ID)

	page, raw := a.clientPage(token)
	require.Len(t, page.Candidates, 1)
	assert.Equal(t, "Wei Tan", page.Candidates[0].Label)
	assert.NotContains(t, raw, "Siti")
	var list api.ClientShortlistList
	a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/client-shortlists", recruiter, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	assert.Len(t, list.Items[0].Candidates, 1, "gone from the recruiter's view too")
}

// Merging a duplicate who is on a client link keeps that place on the job,
// so the client's answer still moves the candidate kept.
func TestAMergedDuplicatesPlaceOnALinkStillMovesTheCandidateKept(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	sj := a.shortlistJob(recruiter, tenant.ID, map[string]map[string]any{"Siti": {}})
	created, token := a.makeShortlist(recruiter, sj.job.Id, map[string]any{"candidates": []any{pick(sj, "Siti")}}, http.StatusCreated)

	keep := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Tan Binti Ali"})
	a.addToJob(recruiter, sj.job.Id, keep.Id) // the kept candidate is on the job too, with nothing attached
	res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", recruiter, map[string]any{"duplicateId": sj.candidates["Siti"].Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	res = a.clientAnswer(token, created.Shortlist.Candidates[0].Id, map[string]any{"decision": "approved"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var apps api.ApplicationList
	a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/applications", recruiter, nil).JSON(&apps)
	require.Len(t, apps.Items, 1, "one place on the job after the merge")
	assert.Equal(t, keep.Id, apps.Items[0].CandidateId)
	assert.Equal(t, api.StageInterview, apps.Items[0].Stage, "the client's approval moved the candidate kept")
}
