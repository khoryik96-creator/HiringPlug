package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func aiConnFor(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(ctx) })
	return conn
}

func withdrawConsent(t *testing.T, a *testApp, token, candidateID string) {
	t.Helper()
	a.setConsent(token, candidateID, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
}

func TestFitIsScoredWithoutBeingAskedWhileTheAIIsOn(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, matchingOn)
	aiConn := aiConnFor(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Auditor", "description": "- At least 3 years in audit"})

	c := a.createCandidate(token, map[string]any{"firstName": "Ali"})
	appl := a.addToJob(token, job.Id, c.Id)
	assert.Empty(t, a.matches(token, "applicationId="+appl.Id).Items, "no CV yet: nothing to score")
	withCV(t, a, aiConn, token, c.Id, "Auditor at KPMG, 2018-2024")
	got := a.matches(token, "applicationId="+appl.Id).Items
	require.Len(t, got, 1, "scored once their CV is read")
	assert.Equal(t, api.MatchStatusQueued, got[0].Status)

	t.Run("not those who withdrew consent", func(t *testing.T) {
		no := a.createCandidate(token, map[string]any{"firstName": "Nur"})
		withCV(t, a, aiConn, token, no.Id, "Auditor at EY, 2019-2024")
		withdrawConsent(t, a, token, no.Id)
		noAppl := a.addToJob(token, job.Id, no.Id)
		assert.Empty(t, a.matches(token, "applicationId="+noAppl.Id).Items)
		res := a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": noAppl.Id})
		assert.Equal(t, api.ErrorCodeMATCHCONSENTWITHDRAWN, res.Error().Error.Code, "nor when asked")
		var r api.MatchJobResult
		a.request(http.MethodPost, "/v1/matches/for-job", token, map[string]any{"jobId": job.Id}).JSON(&r)
		assert.Equal(t, 1, r.ConsentWithdrawn)
	})

	t.Run("a score is outdated once the job's text changes", func(t *testing.T) {
		aiScores(t, a, aiConn, got[0].Id, scoredWith(claim("At least 3 years in audit", api.MatchVerdictMet, "Auditor at KPMG, 2018-2024")))
		assert.False(t, a.matches(token, "applicationId="+appl.Id).Items[0].Outdated)
		var j api.JobOrder
		a.request(http.MethodGet, "/v1/jobs/"+job.Id, token, nil).JSON(&j)
		require.Equal(t, http.StatusOK, a.request(http.MethodPatch, "/v1/jobs/"+job.Id, token,
			map[string]any{"version": j.Version, "description": "- At least 5 years in audit"}).Status)
		m := a.matches(token, "jobId="+job.Id).Items
		require.NotEmpty(t, m)
		for _, x := range m {
			if x.ApplicationId == appl.Id {
				assert.True(t, x.Outdated, "score it again")
			}
		}
	})
}

func TestNothingIsScoredWhileTheAIIsOff(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	aiConn := aiConnFor(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Ali"})
	withCV(t, a, aiConn, token, c.Id, "Auditor at KPMG")
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Auditor", "description": "Audit."})
	a.addToJob(token, job.Id, c.Id)
	assert.Empty(t, a.matches(token, "jobId="+job.Id).Items)
}

func TestPossibleCandidatesShareTheJobsWords(t *testing.T) {
	t.Parallel()
	a := newTestApp(t) // no AI needed
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Senior Auditor",
		"description": "Statutory audit of listed groups, SAP FICO and MFRS reporting. Salary: RM 9,000"})
	cv := func(first, title, text string) api.Candidate {
		c := a.createCandidate(token, map[string]any{"firstName": first, "currentTitle": title})
		withCV(t, a, aiConn, token, c.Id, text)
		return c
	}
	best := cv("Aina", "Auditor", "Senior auditor at KPMG: statutory audit of listed groups, MFRS reporting, SAP FICO")
	some := cv("Ben", "Accountant", "Accountant using SAP for month-end reporting")
	cv("Chris", "Barista", "Barista: coffee, latte art, customer service")
	onJob := cv("Dina", "Auditor", "Statutory audit, MFRS, SAP FICO, listed groups")
	a.addToJob(token, job.Id, onJob.Id)
	noContact := cv("Eva", "Auditor", "Statutory audit, MFRS reporting, SAP FICO, listed groups")
	withdrawConsent(t, a, token, noContact.Id)

	var list api.MatchSuggestionList
	res := a.request(http.MethodGet, "/v1/matches/suggestions?jobId="+job.Id, tenant.Users[api.RoleViewer].Token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	require.Len(t, list.Items, 2, "not the barista, nor who is on the job or withdrew consent")
	assert.Equal(t, best.Id, list.Items[0].CandidateId, "the most shared words first")
	assert.Equal(t, some.Id, list.Items[1].CandidateId)
	assert.Greater(t, list.Items[0].Rank, list.Items[1].Rank)
	assert.Equal(t, "Aina", list.Items[0].CandidateName)
	assert.Equal(t, "Auditor", list.Items[0].CurrentTitle.MustGet())
	assert.True(t, strings.Contains(list.Items[0].Snippet, "[[audit]]") || strings.Contains(list.Items[0].Snippet, "[[statutory]]"), list.Items[0].Snippet)

	bare := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk"})
	res = a.request(http.MethodGet, "/v1/matches/suggestions?jobId="+bare.Id, token, nil)
	assert.Equal(t, api.ErrorCodeMATCHNEEDSTEXT, res.Error().Error.Code)
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/matches/suggestions?jobId="+job.Id, other, nil).Status)
}
