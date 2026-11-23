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

func (a *testApp) activity(token, query string) api.ActivityReport {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/reports/activity"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var r api.ActivityReport
	res.JSON(&r)
	return r
}

func rowOf(t *testing.T, r api.ActivityReport, userID string) api.ActivityRow {
	t.Helper()
	for _, row := range r.Rows {
		if row.UserId == userID {
			return row
		}
	}
	require.Failf(t, "no row", "for %s", userID)
	return api.ActivityRow{}
}

func goal(calls, cvs, interviews, placements any) map[string]any {
	return map[string]any{"calls": calls, "cvsSent": cvs, "interviews": interviews, "placements": placements}
}

func TestActivityAgainstTargets(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	token := recruiter.Token
	client := "hr." + tenant.ID + "@client.test"
	company := a.createCompany(token, map[string]any{"name": "Maju Logistik Sdn Bhd"})
	hr := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman", "email": client})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "contactId": hr.Id, "title": "Hub Operations Manager"})
	farid := a.createCandidate(token, map[string]any{"firstName": "Farid", "email": "farid." + tenant.ID + "@candidates.test"})
	nurul := a.createCandidate(token, map[string]any{"firstName": "Nurul"})

	// Calls: a screening call (its note is the same call), a reference
	// check, a note of a call; a general note is not a call.
	res := a.request(http.MethodPost, "/v1/candidates/"+farid.Id+"/screening-calls", token, map[string]any{"version": farid.Version, "notes": "Wants regional scope"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	for _, kind := range []string{"reference", "call", "general"} {
		res = a.request(http.MethodPost, "/v1/notes", token, map[string]any{"subjectType": "candidate", "subjectId": nurul.Id, "kind": kind, "body": "Spoke today"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	}
	// CVs sent: Farid moved to submitted and his CV emailed the same day
	// counts once; Nurul's CV emailed to the client; Farid's CV emailed to
	// Farid himself is not a CV sent.
	faridApp := a.addToJob(token, job.Id, farid.Id)
	require.Equal(t, http.StatusOK, a.move(token, faridApp, map[string]any{"stage": "submitted"}).Status)
	cvOf := func(c api.Candidate) api.Document {
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", c.FirstName+" CV.pdf", minimalPDF)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var d api.Document
		res.JSON(&d)
		return d
	}
	faridCV, nurulCV := cvOf(farid), cvOf(nurul)
	send := func(to string, candidateID string, cv api.Document) {
		res := a.request(http.MethodPost, "/v1/emails", token, map[string]any{
			"to": []string{to}, "subject": "CV", "bodyHtml": "<p>Please see the CV.</p>", "candidateId": candidateID,
			"cvs": []map[string]any{{"documentId": cv.Id}},
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	}
	send(client, farid.Id, faridCV)
	send(client, nurul.Id, nurulCV)
	send(string(farid.Email.MustGet()), farid.Id, faridCV)
	// An interview arranged and a placement recorded.
	nurulApp := a.addToJob(token, job.Id, nurul.Id)
	a.interview(http.MethodPost, "/v1/applications/"+nurulApp.Id+"/interviews", token, map[string]any{
		"title": "First interview", "startsAt": time.Now().Add(72 * time.Hour), "durationMinutes": 45, "mode": "video",
		"interviewerIds": []string{recruiter.ID},
	}, http.StatusCreated)
	a.placement(http.MethodPost, "/v1/applications/"+faridApp.Id+"/placement", token, map[string]any{
		"startDate": time.Now().UTC().AddDate(0, 1, 0).Format(time.DateOnly),
	}, http.StatusCreated)

	// Owners and admins set the targets; a recruiter may not.
	settings := map[string]any{
		"targets": []map[string]any{
			{"userId": nil, "period": "week", "goal": goal(20, 5, nil, nil)},
			{"userId": recruiter.ID, "period": "month", "goal": goal(80, 20, 8, 2)},
		},
		"stageWeights": []map[string]any{{"stage": "interview", "percent": 40, "monthsAhead": 1}},
	}
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/reports/settings", token, settings).Status)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/reports/settings", token, nil).Status)
	res = a.request(http.MethodPut, "/v1/reports/settings", owner.Token, settings)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var saved api.ReportSettings
	res.JSON(&saved)
	assert.Len(t, saved.Targets, 2)
	require.Len(t, saved.StageWeights, 4, "every stage, the others at their defaults")
	assert.Equal(t, api.StageWeight{Stage: "interview", Percent: 40, MonthsAhead: 1}, saved.StageWeights[1])
	assert.Equal(t, api.StageWeight{Stage: "submitted", Percent: 10, MonthsAhead: 2}, saved.StageWeights[0])

	today := "?day=" + time.Now().In(a.deps.Config.TimeZone).Format(time.DateOnly)
	week := a.activity(token, today)
	assert.False(t, week.Team, "a recruiter sees their own activity only")
	require.Len(t, week.Rows, 1)
	mine := week.Rows[0]
	assert.Equal(t, recruiter.ID, mine.UserId)
	assert.Equal(t, api.ActivityCounts{Calls: 3, CvsSent: 2, Interviews: 1, Placements: 1}, mine.Done)
	target := mine.Target.MustGet()
	assert.Equal(t, 20, target.Calls.MustGet(), "everyone's weekly target")
	assert.True(t, target.Interviews.IsNull())

	month := a.activity(token, today+"&period=month")
	assert.Equal(t, api.ActivityPeriodMonth, month.Period)
	assert.Equal(t, 1, month.From.Day())
	assert.Equal(t, 80, month.Rows[0].Target.MustGet().Calls.MustGet(), "their own monthly target")

	team := a.activity(owner.Token, today)
	assert.True(t, team.Team)
	assert.Equal(t, mine.Done, rowOf(t, team, recruiter.ID).Done)
	assert.Equal(t, api.ActivityCounts{}, rowOf(t, team, owner.ID).Done, "everyone is listed, with nothing done too")
	assert.True(t, rowOf(t, team, owner.ID).Target.IsSpecified())
	for _, row := range team.Rows {
		assert.NotEqual(t, tenant.Users[api.RoleViewer].ID, row.UserId, "a viewer does not recruit: no row, no target")
	}

	lastYear := a.activity(token, "?day=2020-03-04")
	assert.Equal(t, api.ActivityCounts{}, lastYear.Rows[0].Done, "another week")

	t.Run("targets are checked", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/reports/settings", owner.Token, map[string]any{
			"targets": []map[string]any{
				{"userId": recruiter.ID, "period": "week", "goal": goal(1, nil, nil, nil)},
				{"userId": recruiter.ID, "period": "week", "goal": goal(2, nil, nil, nil)},
				{"userId": testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].ID, "period": "week", "goal": goal(1, nil, nil, nil)},
			},
			"stageWeights": []map[string]any{},
		})
		assert.Equal(t, []string{"targets.1.userId", "targets.2.userId"}, fieldNames(t, res.Error()), "twice, and someone from another company")
	})

	other := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	elsewhere := a.activity(other, today)
	require.Len(t, elsewhere.Rows, 1)
	assert.Equal(t, api.ActivityCounts{}, elsewhere.Rows[0].Done, "another company sees nothing of this one")
	assert.True(t, elsewhere.Rows[0].Target.IsNull(), "nor its targets")
}
