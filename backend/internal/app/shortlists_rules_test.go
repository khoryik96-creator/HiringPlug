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

func pick(sj shortlistJob, name string) map[string]any {
	return map[string]any{"applicationId": sj.apps[name].Id, "documentId": sj.cvs[name]}
}

// Only CVs without salary, email or phone go to a client; never someone who
// withdrew consent.
func TestClientShortlistRefusesWhatMustNotReachTheClient(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	sj := a.shortlistJob(recruiter, tenant.ID, map[string]map[string]any{
		"Paid": {"includeSalary": true}, "Contact": {"showContact": true}, "Fine": {}, "Gone": {"blind": true},
	})
	for _, name := range []string{"Paid", "Contact"} {
		res := a.request(http.MethodPost, "/v1/jobs/"+sj.job.Id+"/client-shortlists", recruiter, map[string]any{"candidates": []any{pick(sj, name)}})
		require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s: %s", name, res.Body)
		e := res.Error()
		assert.Equal(t, api.ErrorCodeSHORTLISTCVNOTSHAREABLE, e.Error.Code, name)
		assert.Contains(t, e.Error.Message, name+" Tan")
	}
	// A CV of someone else is not theirs to send.
	wrong := pick(sj, "Fine")
	wrong["documentId"] = sj.cvs["Gone"]
	res := a.request(http.MethodPost, "/v1/jobs/"+sj.job.Id+"/client-shortlists", recruiter, map[string]any{"candidates": []any{wrong}})
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)

	gone := sj.candidates["Gone"]
	a.setConsent(recruiter, gone.Id, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	res = a.request(http.MethodPost, "/v1/jobs/"+sj.job.Id+"/client-shortlists", recruiter, map[string]any{"candidates": []any{pick(sj, "Gone")}})
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeSHORTLISTDONOTCONTACT, res.Error().Error.Code)

	// Someone who withdraws after the link was made drops off it.
	a.setConsent(recruiter, gone.Id, map[string]any{"expectedVersion": 1, "status": "given", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	created, token := a.makeShortlist(recruiter, sj.job.Id, map[string]any{"candidates": []any{pick(sj, "Fine"), pick(sj, "Gone")}}, http.StatusCreated)
	a.setConsent(recruiter, gone.Id, map[string]any{"expectedVersion": 2, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	page, _ := a.clientPage(token)
	require.Len(t, page.Candidates, 1)
	assert.Equal(t, "Fine Tan", page.Candidates[0].Label)
	goneItem := created.Shortlist.Candidates[1].Id
	assert.Equal(t, http.StatusNotFound, a.clientAnswer(token, goneItem, map[string]any{"decision": "approved"}).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/public/client-shortlists/"+token+"/candidates/"+goneItem+"/cv", "", nil).Status)
}

func TestClientShortlistChecksWhatIsAsked(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	sj := a.shortlistJob(recruiter, tenant.ID, map[string]map[string]any{"Siti": {"blind": true}, "Wei": {}})
	path := "/v1/jobs/" + sj.job.Id + "/client-shortlists"

	res := a.request(http.MethodPost, path, recruiter, map[string]any{"candidates": []any{pick(sj, "Siti"), pick(sj, "Siti")}, "expiresInDays": 91})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, path, tenant.Users[api.RoleSourcer].Token,
		map[string]any{"candidates": []any{pick(sj, "Siti")}}).Status, "sourcers do not share with clients")

	// A candidate on another job, or rejected, is not on this shortlist.
	other := a.createJob(recruiter, map[string]any{"companyId": sj.job.CompanyId, "title": "Auditor"})
	res = a.request(http.MethodPost, "/v1/jobs/"+other.Id+"/client-shortlists", recruiter, map[string]any{"candidates": []any{pick(sj, "Siti")}})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"candidates.0.applicationId"}, fieldNames(t, res.Error()))
	res = a.request(http.MethodPost, "/v1/jobs/"+other.Id+"/client-shortlists", recruiter, map[string]any{"candidates": []any{}, "emailHiringContact": true})
	assert.Equal(t, http.StatusBadRequest, res.Status)

	// Emailing needs a hiring contact with an email.
	res = a.request(http.MethodPost, path, recruiter, map[string]any{"candidates": []any{pick(sj, "Wei")}, "emailHiringContact": true})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	wei := a.addToJob(recruiter, other.Id, sj.candidates["Wei"].Id)
	res = a.request(http.MethodPost, "/v1/jobs/"+other.Id+"/client-shortlists", recruiter, map[string]any{
		"candidates": []any{map[string]any{"applicationId": wei.Id, "documentId": sj.cvs["Wei"]}}, "emailHiringContact": true,
	})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"emailHiringContact"}, fieldNames(t, res.Error()))

	// An expired link shows no one and takes no answers.
	created, token := a.makeShortlist(recruiter, sj.job.Id, map[string]any{"candidates": []any{pick(sj, "Siti")}, "expiresInDays": 1}, http.StatusCreated)
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE client_shortlist SET expires_at = now() - interval '1 minute' WHERE id = $1`, created.Shortlist.Id)
	require.NoError(t, err)
	page, _ := a.clientPage(token)
	assert.Equal(t, api.ClientShortlistStatusExpired, page.Status)
	assert.Empty(t, page.Candidates)
	res = a.clientAnswer(token, created.Shortlist.Candidates[0].Id, map[string]any{"decision": "approved"})
	require.Equal(t, http.StatusConflict, res.Status)
	assert.Equal(t, api.ErrorCodeSHORTLISTCLOSED, res.Error().Error.Code)
	var list api.ClientShortlistList
	a.request(http.MethodGet, path, recruiter, nil).JSON(&list)
	assert.True(t, list.Items[0].OpenedAt.IsNull(), "visits to a closed link are not openings")
}

// Approving moves the candidate to the job's own interview stage.
func TestClientShortlistApprovalUsesTheJobsStages(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	sj := a.shortlistJob(recruiter, tenant.ID, map[string]map[string]any{"Siti": {"blind": true}})
	own := []map[string]any{{"label": "New", "stage": "applied"}, {"label": "Sent", "stage": "submitted"},
		{"label": "Client interview", "stage": "interview"}, {"label": "Final interview", "stage": "interview"}, {"label": "Placed", "stage": "placed"}}
	res := a.request(http.MethodPut, "/v1/jobs/"+sj.job.Id+"/stages", recruiter, map[string]any{"version": 0, "steps": own})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var js api.JobStages
	res.JSON(&js)

	created, token := a.makeShortlist(recruiter, sj.job.Id, map[string]any{"candidates": []any{pick(sj, "Siti")}}, http.StatusCreated)
	res = a.clientAnswer(token, created.Shortlist.Candidates[0].Id, map[string]any{"decision": "approved"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var apps api.ApplicationList
	a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/applications", recruiter, nil).JSON(&apps)
	require.Len(t, apps.Items, 1)
	app := apps.Items[0]
	assert.Equal(t, api.StageInterview, app.Stage)
	assert.Equal(t, js.Steps[2].Key, app.Step.MustGet(), "the job's first interview stage")
}
