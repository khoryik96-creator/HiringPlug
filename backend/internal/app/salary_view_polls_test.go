package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A candidate's emails looked at again only for how they are getting on
// (statusOnly: the page asks while one is on its way) come back without
// their texts, and are not counted as a salary read; the list itself is.
func TestEmailsAskedOnlyForTheirStatusAreNotCountedAsRead(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	c := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti", "email": "siti@example.com"})
	res := a.request(http.MethodPost, "/v1/emails", recruiter.Token, map[string]any{"candidateId": c.Id, "to": []string{"hr@client.example"},
		"subject": "Siti", "bodyHtml": "<p>Expecting [Expected salary]</p>", "timeZone": "Asia/Kuala_Lumpur"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	list := func(query string) api.EmailList {
		t.Helper()
		res := a.request(http.MethodGet, "/v1/emails?candidateId="+c.Id+query, recruiter.Token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var out api.EmailList
		res.JSON(&out)
		require.Len(t, out.Items, 1)
		return out
	}

	read := list("")
	assert.Contains(t, read.Items[0].Body, "10,913")
	require.Equal(t, 1, a.seen(owner, recruiter.ID)["email_read:"+c.Id].Count)

	status := list("&statusOnly=true")
	got := status.Items[0]
	assert.Equal(t, read.Items[0].Id, got.Id)
	assert.Equal(t, read.Items[0].Status, got.Status)
	assert.Empty(t, got.Subject)
	assert.Empty(t, got.Body)
	assert.True(t, got.BodyHtml.IsNull() || !got.BodyHtml.IsSpecified(), "no formatted message")
	assert.Equal(t, 1, a.seen(owner, recruiter.ID)["email_read:"+c.Id].Count, "asking how it is getting on reads nothing")
}

// A search by meaning found by the expected salary is asked again while its
// words are read: those answers find no one yet, and count nothing; the
// answer with the candidates counts them once.
func TestASearchByMeaningBeingReadCountsNoSalary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	a.withSalary(recruiter.Token, map[string]any{"firstName": "Aina", "currentTitle": "Talent Acquisition Partner"})
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/meaning/status", recruiter.Token, nil).Status) // starts the read-through
	require.Equal(t, 1, readByMeaning(t, a, aiConn, tenant.ID))
	search := func() api.CandidateSearchPage {
		t.Helper()
		res := a.request(http.MethodPost, "/v1/candidates/search", recruiter.Token, map[string]any{
			"meaning": "recruiter", "expectedSalaryMax": map[string]any{"amount": "20000", "currency": "MYR"}})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var page api.CandidateSearchPage
		res.JSON(&page)
		return page
	}

	for range 3 { // the page asks again while the words are read
		page := search()
		outcome, err := page.Meaning.Get()
		require.NoError(t, err)
		require.Equal(t, api.MeaningOutcomeStatusReading, outcome.Status)
		assert.Empty(t, page.Items)
	}
	assert.Empty(t, a.seen(owner, recruiter.ID), "no one found yet: nothing told about a salary")
	_, ok := readSearch(t, a, aiConn, tenant.ID, api.MeaningUnderstood{Role: str("recruiter"), Family: str("recruitment")})
	require.True(t, ok)
	page := search()
	require.Len(t, page.Items, 1)
	assert.Equal(t, 1, a.seen(owner, recruiter.ID)["candidate_list:"].Count, "found by the expected salary: counted once")
	assert.Equal(t, 1, a.seen(owner, recruiter.ID)["candidate_list:"].Candidates)
}
