package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) addToJob(token, jobID, candidateID string) api.Application {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/jobs/"+jobID+"/applications", token, map[string]any{"candidateId": candidateID})
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var app api.Application
	res.JSON(&app)
	return app
}

func (a *testApp) move(token string, app api.Application, body map[string]any) *response {
	a.t.Helper()
	body["version"] = app.Version
	return a.request(http.MethodPost, "/v1/applications/"+app.Id+"/move", token, body)
}

func TestPipelineAddAndMove(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maybank"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Auditor"})
	siti := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "currentTitle": "Accountant"})

	app := a.addToJob(token, job.Id, siti.Id)
	assert.Equal(t, api.StageApplied, app.Stage)
	assert.Equal(t, "Auditor", app.JobTitle)
	assert.Equal(t, "Maybank", app.CompanyName)
	assert.Equal(t, "Siti Aisyah", app.CandidateName)
	title, _ := app.CandidateTitle.Get()
	assert.Equal(t, "Accountant", title)

	res := a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/applications", token, map[string]any{"candidateId": siti.Id})
	require.Equal(t, http.StatusConflict, res.Status, "once per job")
	assert.Equal(t, api.ErrorCodeAPPLICATIONEXISTS, res.Error().Error.Code)
	assert.Equal(t, app.Id, (*res.Error().Error.Details)["existingApplicationId"])

	res = a.move(token, app, map[string]any{"stage": "interview"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var moved api.Application
	res.JSON(&moved)
	assert.Equal(t, api.StageInterview, moved.Stage)
	assert.Equal(t, 2, moved.Version)
	assert.True(t, moved.StageChangedAt.After(app.StageChangedAt) || moved.StageChangedAt.Equal(app.StageChangedAt))

	res = a.move(token, app, map[string]any{"stage": "offer"})
	assert.Equal(t, http.StatusConflict, res.Status, "two people moving the same card: the second is told")

	res = a.move(token, moved, map[string]any{"stage": "rejected"})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"reason"}, fieldNames(t, res.Error()))

	res = a.move(token, moved, map[string]any{"stage": "rejected", "reason": "failed_interview", "reasonNote": " Weak on IFRS "})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var rejected api.Application
	res.JSON(&rejected)
	reason, _ := rejected.Reason.Get()
	note, _ := rejected.ReasonNote.Get()
	assert.Equal(t, api.CloseReasonFailedInterview, reason)
	assert.Equal(t, "Weak on IFRS", note)

	res = a.move(token, rejected, map[string]any{"stage": "placed"})
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeSTAGEMOVENOTALLOWED, res.Error().Error.Code)

	res = a.move(token, rejected, map[string]any{"stage": "offer"})
	require.Equal(t, http.StatusOK, res.Status, "reopened")
	var reopened api.Application
	res.JSON(&reopened)
	assert.True(t, reopened.Reason.IsNull(), "the reason goes when reopened")
}

func TestPipelineListsForJobAndCandidate(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer, api.RoleSourcer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Grab"})
	backend := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Backend Engineer"})
	data := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Data Engineer"})
	wei := a.createCandidate(token, map[string]any{"firstName": "Wei"})
	mei := a.createCandidate(token, map[string]any{"firstName": "Mei"})
	a.addToJob(token, backend.Id, wei.Id)
	a.addToJob(tenant.Users[api.RoleSourcer].Token, backend.Id, mei.Id) // sourcers add candidates to jobs
	a.addToJob(token, data.Id, wei.Id)

	var list api.ApplicationList
	a.request(http.MethodGet, "/v1/jobs/"+backend.Id+"/applications", tenant.Users[api.RoleViewer].Token, nil).JSON(&list)
	require.Len(t, list.Items, 2)
	assert.Equal(t, "Wei", list.Items[0].CandidateName, "in the order added")
	assert.Equal(t, "Mei", list.Items[1].CandidateName)
	require.NotNil(t, list.Items[0].CandidateFirstName)
	assert.Equal(t, "Wei", *list.Items[0].CandidateFirstName, "the board shows first names")

	a.request(http.MethodGet, "/v1/candidates/"+wei.Id+"/applications", token, nil).JSON(&list)
	require.Len(t, list.Items, 2)
	assert.Equal(t, "Data Engineer", list.Items[0].JobTitle, "newest first")
	assert.Equal(t, "Grab", list.Items[0].CompanyName)

	res := a.request(http.MethodPost, "/v1/jobs/"+data.Id+"/applications", tenant.Users[api.RoleViewer].Token, map[string]any{"candidateId": mei.Id})
	assert.Equal(t, http.StatusForbidden, res.Status, "viewers may look, not change")

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	res = a.request(http.MethodGet, "/v1/jobs/"+backend.Id+"/applications", other.Users[api.RoleOwner].Token, nil)
	assert.Equal(t, http.StatusNotFound, res.Status, "another company's job does not exist for you")
	otherJob := a.createJob(other.Users[api.RoleOwner].Token, map[string]any{
		"companyId": a.createCompany(other.Users[api.RoleOwner].Token, map[string]any{"name": "Elsewhere"}).Id, "title": "X",
	})
	res = a.request(http.MethodPost, "/v1/jobs/"+otherJob.Id+"/applications", other.Users[api.RoleOwner].Token, map[string]any{"candidateId": wei.Id})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"candidateId"}, fieldNames(t, res.Error()), "another company's candidate does not exist for you")
}
