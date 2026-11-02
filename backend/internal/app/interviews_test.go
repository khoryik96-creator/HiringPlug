package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/humantime"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) interview(method, path, token string, body any, want int) api.Interview {
	a.t.Helper()
	res := a.request(method, path, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var iv api.Interview
	res.JSON(&iv)
	return iv
}

func TestInterviews(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager, api.RoleSourcer, api.RoleViewer)
	recruiter, manager, sourcer := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleHiringManager], tenant.Users[api.RoleSourcer]
	company := a.createCompany(recruiter.Token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	cand := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	app := a.addToJob(recruiter.Token, job.Id, cand.Id)
	path := "/v1/applications/" + app.Id + "/interviews"
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	body := map[string]any{
		"title": "First interview", "startsAt": start, "durationMinutes": 60, "mode": "video",
		"location": "https://meet.example/abc", "interviewerIds": []string{manager.ID, sourcer.ID},
	}

	users := a.request(http.MethodGet, "/v1/users", manager.Token, nil).Map()["items"].([]any)
	assert.Len(t, users, 4, "the company's users, to pick interviewers")
	for _, u := range users {
		m := u.(map[string]any)
		assert.Equal(t, m["role"] != "viewer", m["canInterview"], "viewers cannot give feedback")
	}

	iv := a.interview(http.MethodPost, path, recruiter.Token, body, http.StatusCreated)
	assert.Equal(t, api.InterviewStatusScheduled, iv.Status)
	assert.Equal(t, "Siti Aisyah", iv.CandidateName)
	assert.Equal(t, "Accountant", iv.JobTitle)
	assert.Len(t, iv.Interviewers, 2)
	stageOf := func(id string) (string, float64) {
		for _, item := range a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter.Token, nil).Map()["items"].([]any) {
			if m := item.(map[string]any); m["id"] == id {
				return m["stage"].(string), m["version"].(float64)
			}
		}
		t.Fatalf("application %s not found", id)
		return "", 0
	}
	stage, version := stageOf(app.Id)
	assert.Equal(t, "interview", stage, "scheduling moves the application to the interview stage")

	t.Run("a busy interviewer is a clash, unless asked to ignore it", func(t *testing.T) {
		other := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei"}).Id)
		overlap := map[string]any{
			"title": "First interview", "startsAt": start.Add(30 * time.Minute), "durationMinutes": 30, "mode": "phone",
			"interviewerIds": []string{manager.ID},
		}
		res := a.request(http.MethodPost, "/v1/applications/"+other.Id+"/interviews", recruiter.Token, overlap)
		assert.Equal(t, http.StatusConflict, res.Status)
		assert.Contains(t, string(res.Body), "INTERVIEW_CLASH")
		overlap["ignoreClashes"] = true
		a.interview(http.MethodPost, "/v1/applications/"+other.Id+"/interviews", recruiter.Token, overlap, http.StatusCreated)
	})

	t.Run("checks interviewers and who may schedule", func(t *testing.T) {
		stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter]
		bad := map[string]any{"title": "x", "startsAt": start, "durationMinutes": 30, "mode": "phone", "interviewerIds": []string{stranger.ID}}
		res := a.request(http.MethodPost, path, recruiter.Token, bad)
		assert.Equal(t, http.StatusBadRequest, res.Status, "another company's user is not an interviewer here")
		viewer := map[string]any{"title": "x", "startsAt": start, "durationMinutes": 30, "mode": "phone", "interviewerIds": []string{tenant.Users[api.RoleViewer].ID}}
		res = a.request(http.MethodPost, path, recruiter.Token, viewer)
		assert.Equal(t, http.StatusBadRequest, res.Status)
		assert.Contains(t, string(res.Body), "cannot give interview feedback", "viewers cannot be interviewers")
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, path, manager.Token, body).Status, "hiring managers interview, recruiters schedule")
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/interviews/"+iv.Id, stranger.Token, nil).Status)
	})

	// It took place: move it into the past.
	_, err := testutil.Current.Admin.Exec(t.Context(), `UPDATE interview SET starts_at = now() - interval '2 hours', ends_at = now() - interval '1 hour' WHERE id = $1`, iv.Id)
	require.NoError(t, err)

	due := a.request(http.MethodGet, "/v1/interviews?feedbackDue=true", manager.Token, nil).Map()["items"].([]any)
	require.Len(t, due, 1, "the manager owes feedback")
	assert.Equal(t, iv.Id, due[0].(map[string]any)["id"])

	given := a.interview(http.MethodPut, "/v1/interviews/"+iv.Id+"/feedback", manager.Token,
		map[string]any{"recommendation": "yes", "comments": "Strong on month-end close."}, http.StatusOK)
	require.Len(t, given.Feedback, 1)
	assert.False(t, given.MyFeedbackDue)

	t.Run("an interviewer sees colleagues' feedback only after giving their own", func(t *testing.T) {
		before := a.interview(http.MethodGet, "/v1/interviews/"+iv.Id, sourcer.Token, nil, http.StatusOK)
		assert.Empty(t, before.Feedback)
		assert.Equal(t, 1, before.FeedbackHidden)
		assert.True(t, before.MyFeedbackDue)
		after := a.interview(http.MethodPut, "/v1/interviews/"+iv.Id+"/feedback", sourcer.Token, map[string]any{"recommendation": "no"}, http.StatusOK)
		assert.Len(t, after.Feedback, 2)
		assert.Zero(t, after.FeedbackHidden)
		assert.Len(t, a.interview(http.MethodGet, "/v1/interviews/"+iv.Id, recruiter.Token, nil, http.StatusOK).Feedback, 2, "recruiters see all")
	})

	t.Run("only interviewers give feedback", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/interviews/"+iv.Id+"/feedback", recruiter.Token, map[string]any{"recommendation": "yes"})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
		assert.Contains(t, string(res.Body), "Only the interview's interviewers")
	})

	t.Run("reschedule, complete, and the calendar file", func(t *testing.T) {
		done := a.interview(http.MethodPatch, "/v1/interviews/"+iv.Id, recruiter.Token, map[string]any{"version": iv.Version, "status": "completed"}, http.StatusOK)
		assert.Equal(t, api.InterviewStatusCompleted, done.Status)
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPatch, "/v1/interviews/"+iv.Id, recruiter.Token, map[string]any{"version": iv.Version, "title": "x"}).Status)
		ics := a.request(http.MethodGet, "/v1/interviews/"+iv.Id+"/calendar.ics", manager.Token, nil)
		require.Equal(t, http.StatusOK, ics.Status)
		assert.Contains(t, ics.Header.Get("Content-Type"), "text/calendar")
		assert.True(t, strings.HasPrefix(string(ics.Body), "BEGIN:VCALENDAR"))
		assert.Contains(t, string(ics.Body), "SUMMARY:First interview: Siti Aisyah (Accountant)")
	})

	mine := a.request(http.MethodGet, "/v1/interviews?mine=true", sourcer.Token, nil).Map()["items"].([]any)
	assert.Len(t, mine, 1)

	t.Run("closed applications get no new interviews", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/applications/"+app.Id+"/move", recruiter.Token, map[string]any{"version": int(version), "stage": "rejected", "reason": "not_qualified"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		late := map[string]any{"title": "Second", "startsAt": start.Add(24 * time.Hour), "durationMinutes": 30, "mode": "phone", "interviewerIds": []string{manager.ID}}
		res = a.request(http.MethodPost, path, recruiter.Token, late)
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
		assert.Contains(t, string(res.Body), "reopen it")
	})
}

func TestInterviewReminders(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager, api.RoleSourcer)
	recruiter, manager, sourcer := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleHiringManager], tenant.Users[api.RoleSourcer]
	managerEmail := "hiring_manager." + tenant.ID + "@test.local"
	company := a.createCompany(recruiter.Token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	app := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei"}).Id)
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Minute)
	iv := a.interview(http.MethodPost, "/v1/applications/"+app.Id+"/interviews", recruiter.Token, map[string]any{
		"title": "First interview", "startsAt": start, "durationMinutes": 60, "mode": "phone", "interviewerIds": []string{manager.ID, sourcer.ID},
	}, http.StatusCreated)

	ctx := context.Background()
	var runAfter time.Time
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx,
		`SELECT run_after FROM job WHERE tenant_id = $1 AND kind = 'interview.remind'`, tenant.ID).Scan(&runAfter))
	assert.WithinDuration(t, start.Add(-24*time.Hour), runAfter, time.Second, "the reminder waits until the day before")

	moved := start.Add(2 * time.Hour)
	a.interview(http.MethodPatch, "/v1/interviews/"+iv.Id, recruiter.Token, map[string]any{"version": iv.Version, "startsAt": moved}, http.StatusOK)
	other := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Aina"}).Id)
	cancelled := a.interview(http.MethodPost, "/v1/applications/"+other.Id+"/interviews", recruiter.Token, map[string]any{
		"title": "Panel", "startsAt": start.Add(24 * time.Hour), "durationMinutes": 30, "mode": "video", "interviewerIds": []string{manager.ID},
	}, http.StatusCreated)
	a.interview(http.MethodPatch, "/v1/interviews/"+cancelled.Id, recruiter.Token, map[string]any{"version": cancelled.Version, "status": "cancelled"}, http.StatusOK)

	// The day before comes: every reminder is due.
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'interview.remind'`, tenant.ID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	msgs := mailServer.To(managerEmail)
	require.Len(t, msgs, 1, "one reminder: at the new time only, and none for the cancelled interview")
	assert.ElementsMatch(t, []string{managerEmail, "sourcer." + tenant.ID + "@test.local"}, msgs[0].To)
	assert.Contains(t, msgs[0].Data, "You are interviewing Wei for Accountant.")
	assert.Contains(t, msgs[0].Data, "Test hiring_manager, Test sourcer")
	assert.Contains(t, msgs[0].Data, humantime.When(moved, kualaLumpur(t)), "in the company's time zone")

	// A second run of the same reminder (a retry) does not send it again.
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET status = 'pending', run_after = now() WHERE tenant_id = $1 AND kind = 'interview.remind'`, tenant.ID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Len(t, mailServer.To(managerEmail), 1)
}

func kualaLumpur(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	return loc
}
