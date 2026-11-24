package app_test

// Search by meaning's background reading and what a search does with it:
// counted as it goes (not on every search), old searches forgotten, a
// reading overtaken by a change not saved, a server with no model asked
// nothing; one word of a search found in CVs; an export of the matches.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// runSweepNow runs the company's read-through's next run now (it waits a
// few seconds between runs, and ten minutes between passes).
func runSweepNow(t *testing.T, a *testApp, tenantID string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE job SET run_after = now()
		WHERE tenant_id = $1 AND kind = 'meaning.sweep' AND status = 'pending'`, tenantID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenantID)
}

func meaningStatus(t *testing.T, a *testApp, token string) api.MeaningStatus {
	t.Helper()
	res := a.request(http.MethodGet, "/v1/meaning/status", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var status api.MeaningStatus
	res.JSON(&status)
	return status
}

// How far reading has got is counted by the read-through as it goes, not
// by every search (counting a large company takes seconds); a pass forgets
// the searches nobody has used for 90 days.
func TestTheReadThroughCountsAsItGoesAndForgetsOldSearches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	a.createCandidate(token, map[string]any{"firstName": "Aina", "currentTitle": "Recruiter"})
	a.createCandidate(token, map[string]any{"firstName": "Ben", "currentTitle": "Accountant"})

	status := meaningStatus(t, a, token)
	assert.Equal(t, 2, status.Total, "before the first count: counted now")
	require.Equal(t, 2, readByMeaning(t, a, aiConn, tenant.ID))
	status = meaningStatus(t, a, token)
	assert.Equal(t, [2]int{2, 2}, [2]int{status.Read, status.Total}, "counted when the pass began, and each read since")

	// Someone added behind its back (an import): counted when it counts again, not by a search.
	_, err = testutil.Current.Admin.Exec(ctx, `INSERT INTO candidate (tenant_id, first_name, created_by) VALUES ($1, 'Chong', $2)`,
		tenant.ID, tenant.Users[api.RoleRecruiter].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, meaningStatus(t, a, token).Total)

	var old string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `INSERT INTO meaning_query (tenant_id, text_hash, body, status, used_at)
		VALUES ($1, repeat('a', 64), 'payroll in 2025', 'ready', now() - interval '91 days') RETURNING id`, tenant.ID).Scan(&old))
	for range 3 { // to the end of the pass, then the next begins
		runSweepNow(t, a, tenant.ID)
	}
	var left int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM meaning_query WHERE id = $1`, old).Scan(&left))
	assert.Zero(t, left, "a search unused for 90 days is forgotten")
	assert.Equal(t, 3, meaningStatus(t, a, token).Total, "counted again as the next pass began")
}

// A reading overtaken by a change (a new CV, a new title) while it was being
// read is not saved over the newer one; and a server without the meaning
// model is not asked to read each new CV.
func TestAnOutOfDateReadingIsNotSavedAndNoModelIsNotAsked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Aina", "currentTitle": "Clerk"})
	meaningStatus(t, a, token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	job, ok := claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadCandidates)
	require.True(t, ok)
	var asked api.MeaningReadCandidatesJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))

	res := a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": c.Version, "currentTitle": "Recruiter"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	v := fakeMeaning("Clerk")
	answerAIJob(t, a, aiConn, job, api.JobKindMeaningCandidatesRead, api.MeaningCandidatesReadJob{Model: "stand-in", Candidates: []api.MeaningCandidateRead{{
		CandidateId: c.Id, Fingerprint: asked.Candidates[0].Fingerprint, Profile: &v, Passages: []api.MeaningPassageRead{},
		Roles: []api.MeaningRoleRead{{Ordinal: 0, Title: "Clerk", Field: v, Work: v}},
	}}})
	var saved int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM meaning_candidate WHERE candidate_id = $1`, c.Id).Scan(&saved))
	assert.Zero(t, saved, "read as a Clerk, but a Recruiter by now: not saved")
	runSweepNow(t, a, tenant.ID)
	job, ok = claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadCandidates)
	require.True(t, ok, "read again as they are now")
	assert.Contains(t, string(job.Payload), "Recruiter")

	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE meaning_sweep SET unavailable_until = now() + interval '1 hour' WHERE tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
	ben := a.createCandidate(token, map[string]any{"firstName": "Ben"})
	withCV(t, a, aiConn, token, ben.Id, "Recruiter. Hiring and sourcing.")
	_, ok = claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadCandidates)
	assert.False(t, ok, "no meaning model on this server: a new CV is not sent to be read")
}

// One word searched by meaning ("headhunter", "ACCA") is found in CVs on
// its own (two words were needed), and why says the CV names it.
func TestOneWordSearchedByMeaningIsFoundInCVs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	ben := a.createCandidate(token, map[string]any{"firstName": "Ben", "currentTitle": "Recruitment Consultant"})
	withCV(t, a, aiConn, token, ben.Id, "Recruitment Consultant. Headhunter for MNC clients; 30 placements a year.")
	startReading(t, a, token)
	readByMeaning(t, a, aiConn, tenant.ID)

	page := searchByMeaning(t, a, aiConn, tenant.ID, token, map[string]any{"meaning": "headhunter"},
		api.MeaningUnderstood{Role: str("headhunter"), Family: str("recruitment")})
	require.Equal(t, []string{"Ben"}, names(page))
	why, err := page.Items[0].Why.Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Headhunter"}, why.Words, "why: their CV names the word searched")
}

// Export while searching by meaning gives the best matches the page shows,
// not everyone; and says so while the search is still being read.
func TestExportingASearchByMeaningGivesItsMatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	a.createCandidate(token, map[string]any{"firstName": "Aina", "currentTitle": "Talent Acquisition Partner"})
	a.createCandidate(token, map[string]any{"firstName": "Chong", "currentTitle": "Accountant"})
	startReading(t, a, token)
	readByMeaning(t, a, aiConn, tenant.ID)

	export := func() (int, string) {
		res := a.request(http.MethodPost, "/v1/candidates/export", token, map[string]any{"format": "csv", "search": map[string]any{"meaning": "recruiter"}})
		return res.Status, string(res.Body)
	}
	status, body := export()
	assert.Equal(t, http.StatusConflict, status, "the search is still being read: %s", body)
	assert.Contains(t, body, string(api.ErrorCodeMEANINGREADING))
	searchByMeaning(t, a, aiConn, tenant.ID, token, map[string]any{"meaning": "recruiter"}, api.MeaningUnderstood{Role: str("recruiter"), Family: str("recruitment")})
	status, body = export()
	require.Equal(t, http.StatusOK, status, "%s", body)
	assert.Contains(t, body, "Aina")
	assert.NotContains(t, body, "Chong", "only the matches, not everyone")
}

// The CV text around a boolean's words is shown only to those who may read
// CVs (the search still finds by it).
func TestCVSnippetsNeedThePermissionToReadCVs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Aina"})
	withCV(t, a, aiConn, recruiter, c.Id, "Recruiter. Divorced, two children; expected salary RM 9,000.")

	search := func() api.CandidateSearchPage {
		res := a.request(http.MethodPost, "/v1/candidates/search", recruiter, map[string]any{"boolean": "divorced"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var page api.CandidateSearchPage
		res.JSON(&page)
		return page
	}
	page := search()
	require.Len(t, page.Items, 1)
	assert.NotEmpty(t, page.Items[0].CvSnippet, "a recruiter may read CVs")

	var without []string
	for _, perm := range authz.PermissionsForRole(api.RoleRecruiter) {
		if perm != api.PermissionDocumentsRead {
			without = append(without, string(perm))
		}
	}
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": without, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	page = search()
	require.Len(t, page.Items, 1, "still found by their CV")
	snippet, err := page.Items[0].CvSnippet.Get()
	assert.True(t, err != nil || snippet == "", "but none of the CV's text is shown: %q", snippet)
	assert.False(t, strings.Contains(string(a.request(http.MethodPost, "/v1/candidates/search", recruiter,
		map[string]any{"boolean": "divorced"}).Body), "Divorced"))
}
