package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) jobStages(token, jobID string) api.JobStages {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/jobs/"+jobID+"/stages", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var js api.JobStages
	res.JSON(&js)
	return js
}

func labels(steps []api.StageStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Label
	}
	return out
}

var techSteps = []map[string]any{
	{"label": "Applied", "stage": "applied"}, {"label": "Phone screen", "stage": "screening"},
	{"label": "Tech test", "stage": "interview"}, {"label": "Panel", "stage": "interview"},
	{"label": "Offer", "stage": "offer"}, {"label": "Placed", "stage": "placed"},
}

// Stage templates: saved, changed and deleted by owners and admins; the
// default is what new jobs follow; a job that chose one keeps its copy.
func TestStageTemplates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin, recruiter := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Maju"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Developer"})

	js := a.jobStages(recruiter, job.Id)
	assert.Equal(t, api.JobStagesSourceStandard, js.Source)
	assert.Equal(t, []string{"Applied", "Screening", "Submitted", "1st Interview", "2nd Interview", "Final Interview", "Offer", "Placed"}, labels(js.Steps))

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/stage-templates", recruiter, map[string]any{"name": "x", "steps": techSteps}).Status, "owners and admins")
	bad := a.request(http.MethodPost, "/v1/stage-templates", admin, map[string]any{"name": " ", "steps": []map[string]any{{"label": "Offer", "stage": "offer"}, {"label": "Screen", "stage": "screening"}}})
	require.Equal(t, http.StatusBadRequest, bad.Status)
	assert.ElementsMatch(t, []string{"name", "steps.1.stage"}, fieldNames(t, bad.Error()))

	res := a.request(http.MethodPost, "/v1/stage-templates", admin, map[string]any{"name": "Tech hiring", "isDefault": true, "steps": techSteps})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var tech api.StageTemplate
	res.JSON(&tech)
	assert.True(t, tech.IsDefault)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/stage-templates", admin, map[string]any{"name": "tech HIRING", "steps": techSteps}).Status, "names are unique")

	js = a.jobStages(recruiter, job.Id)
	assert.Equal(t, api.JobStagesSourceDefault, js.Source, "the job follows the default")
	assert.Equal(t, labels(tech.Steps), labels(js.Steps))
	assert.Equal(t, "Tech hiring", js.TemplateName.MustGet())

	t.Run("candidates move to the job's own stages", func(t *testing.T) {
		app := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti"}).Id)
		panel := tech.Steps[3].Key
		res := a.move(recruiter, app, map[string]any{"stage": "offer", "step": panel})
		require.Equal(t, http.StatusBadRequest, res.Status, "a stage of another kind")
		res = a.move(recruiter, app, map[string]any{"stage": "interview", "step": panel})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&app)
		assert.Equal(t, api.StageInterview, app.Stage, "reports go by the kind")
		assert.Equal(t, panel, app.Step.MustGet())
		res = a.move(recruiter, app, map[string]any{"stage": "interview", "step": tech.Steps[2].Key})
		require.Equal(t, http.StatusOK, res.Status, "back to the other interview stage: %s", res.Body)
	})

	t.Run("a job keeps its own copy; changing or deleting the template leaves it", func(t *testing.T) {
		other := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Analyst"})
		res := a.request(http.MethodPut, "/v1/jobs/"+other.Id+"/stages", recruiter, map[string]any{"version": 0, "templateId": tech.Id})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var chosen api.JobStages
		res.JSON(&chosen)
		assert.Equal(t, api.JobStagesSourceTemplate, chosen.Source)

		edited := append([]map[string]any{}, techSteps...)
		edited[1] = map[string]any{"key": tech.Steps[1].Key, "label": "Call", "stage": "screening"}
		res = a.request(http.MethodPut, "/v1/stage-templates/"+tech.Id, admin, map[string]any{"version": tech.Version, "name": "Tech hiring", "isDefault": true, "steps": edited})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&tech)
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, "/v1/stage-templates/"+tech.Id, admin, map[string]any{"version": 1, "name": "Tech hiring", "steps": edited}).Status, "stale")
		assert.Contains(t, labels(a.jobStages(recruiter, job.Id).Steps), "Call", "the job following the default sees the change")
		assert.Contains(t, labels(a.jobStages(recruiter, other.Id).Steps), "Phone screen", "the job that chose it keeps its copy")

		var list api.StageTemplateList
		a.request(http.MethodGet, "/v1/stage-templates", recruiter, nil).JSON(&list)
		require.Len(t, list.Items, 1)
		assert.Equal(t, 1, list.Items[0].JobCount)
		assert.Len(t, list.Standard, 8, "one per category")

		require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/stage-templates/"+tech.Id, admin, nil).Status)
		kept := a.jobStages(recruiter, other.Id)
		assert.Equal(t, api.JobStagesSourceCustom, kept.Source)
		assert.Contains(t, labels(kept.Steps), "Phone screen", "kept after the template went")
		assert.Equal(t, api.JobStagesSourceStandard, a.jobStages(recruiter, job.Id).Source, "no default any more")
	})

	t.Run("a job's own stages, and back to the default", func(t *testing.T) {
		js := a.jobStages(recruiter, job.Id)
		own := []map[string]any{{"label": "New", "stage": "applied"}, {"label": "Client interview", "stage": "interview"}, {"label": "Placed", "stage": "placed"}}
		res := a.request(http.MethodPut, "/v1/jobs/"+job.Id+"/stages", recruiter, map[string]any{"version": js.Version, "steps": own})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&js)
		assert.Equal(t, api.JobStagesSourceCustom, js.Source)
		assert.Equal(t, []string{"New", "Client interview", "Placed"}, labels(js.Steps))
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, "/v1/jobs/"+job.Id+"/stages", recruiter, map[string]any{"version": 0, "steps": own}).Status, "stale")
		res = a.request(http.MethodPut, "/v1/jobs/"+job.Id+"/stages", recruiter, map[string]any{"version": js.Version})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&js)
		assert.Equal(t, api.JobStagesSourceStandard, js.Source)
		assert.Zero(t, js.Version)
	})
}
