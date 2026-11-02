package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMergingDuplicateCandidates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	token := tenant.Users[api.RoleRecruiter].Token
	keep := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "currentTitle": "Accountant"})
	dup := a.createCandidate(token, map[string]any{
		"firstName": "Siti", "lastName": "Aishah", "email": "siti@example.com", "phone": "012-345 6789", "currentTitle": "Clerk",
	})
	// The duplicate has a CV, a salary, a note and two jobs (one the kept candidate is on too).
	require.Equal(t, http.StatusCreated, a.upload("/v1/candidates/"+dup.Id+"/documents", token, "original", "cv.pdf", minimalPDF).Status)
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+dup.Id+"/compensation", token, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "6000", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)
	require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/notes", token, map[string]any{"subjectType": "candidate", "subjectId": dup.Id, "body": "Called her."}).Status)
	company := a.createCompany(token, map[string]any{"name": "Maybank"})
	shared := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Auditor"})
	other := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Analyst"})
	a.addToJob(token, shared.Id, keep.Id)
	a.addToJob(token, shared.Id, dup.Id)
	a.addToJob(token, other.Id, dup.Id)

	res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", tenant.Users[api.RoleSourcer].Token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	assert.Equal(t, http.StatusForbidden, res.Status, "sourcers cannot see salaries, so they cannot merge")
	res = a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": keep.Id, "version": keep.Version})
	assert.Equal(t, http.StatusBadRequest, res.Status, "not into itself")

	res = a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var merged api.Candidate
	res.JSON(&merged)
	email, _ := merged.Email.Get()
	title, _ := merged.CurrentTitle.Get()
	assert.Equal(t, "siti@example.com", string(email), "empty fields are filled from the duplicate")
	assert.Equal(t, "Accountant", title, "the kept candidate's values win")
	comp, err := merged.Compensation.Get()
	require.NoError(t, err)
	current, _ := comp.CurrentSalary.Get()
	assert.Equal(t, "6000.00", current.Amount, "the salary moved")

	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/candidates/"+dup.Id, token, nil).Status, "the duplicate is gone")
	var docs api.DocumentList
	a.request(http.MethodGet, "/v1/candidates/"+keep.Id+"/documents", token, nil).JSON(&docs)
	assert.Len(t, docs.Items, 1, "the CV moved")
	var notes api.NoteList
	a.request(http.MethodGet, "/v1/notes?subjectType=candidate&subjectId="+keep.Id, token, nil).JSON(&notes)
	assert.Len(t, notes.Items, 1, "the note moved")
	var apps api.ApplicationList
	a.request(http.MethodGet, "/v1/candidates/"+keep.Id+"/applications", token, nil).JSON(&apps)
	assert.Len(t, apps.Items, 2, "both jobs, once each")
	var activity api.ActivityList
	a.request(http.MethodGet, "/v1/activity?entityType=candidate&entityId="+keep.Id, token, nil).JSON(&activity)
	assert.Equal(t, "candidate.merged", activity.Items[0].Action)
}
