package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func names(page api.CandidateSearchPage) []string {
	out := []string{}
	for _, hit := range page.Items {
		out = append(out, strings.Fields(hit.Candidate.FullName)[0])
	}
	return out
}

func str(s string) *string { return &s }

// A search by meaning finds people by what they do, not only by the words
// their CVs use, best first, says why, and keeps to the other filters; the
// company's candidates are read in the background, and the same words are
// read once.
func TestCandidatesAreFoundByMeaningNotOnlyByTheirWords(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	a.createCandidate(token, map[string]any{"firstName": "Aina", "currentTitle": "Talent Acquisition Partner", "location": "Kuala Lumpur"})
	ben := a.createCandidate(token, map[string]any{"firstName": "Ben", "currentTitle": "Principal Consultant"})
	withCV(t, a, aiConn, token, ben.Id, "Principal Consultant. Made 25 placements a year: hiring for MNC clients, sourcing and headhunting.")
	a.createCandidate(token, map[string]any{"firstName": "Chong", "currentTitle": "Accountant"})
	a.createCandidate(token, map[string]any{"firstName": "Dewi"})

	var status api.MeaningStatus
	res := a.request(http.MethodGet, "/v1/meaning/status", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&status)
	assert.True(t, status.Available)
	assert.Equal(t, 4, status.Total)
	assert.Zero(t, status.Read, "not read yet: the read-through has just started")
	assert.Equal(t, 4, readByMeaning(t, a, aiConn, tenant.ID), "everyone, those with nothing to read too (so they are not queued again)")
	a.request(http.MethodGet, "/v1/meaning/status", token, nil).JSON(&status)
	assert.Equal(t, 4, status.Read)

	search := func(body map[string]any) api.CandidateSearchPage {
		res := a.request(http.MethodPost, "/v1/candidates/search", token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var page api.CandidateSearchPage
		res.JSON(&page)
		return page
	}
	page := search(map[string]any{"meaning": "recruiter"})
	outcome, err := page.Meaning.Get()
	require.NoError(t, err)
	assert.Equal(t, api.MeaningOutcomeStatusReading, outcome.Status, "the words are read first (a moment)")
	assert.Empty(t, page.Items)
	asked, ok := readSearch(t, a, aiConn, tenant.ID, api.MeaningUnderstood{Role: str("recruiter"), Family: str("recruitment")})
	require.True(t, ok)
	assert.Equal(t, "recruiter", asked.Body)
	assert.Nil(t, asked.Title, "a search, not a job")

	page = search(map[string]any{"meaning": "  Recruiter "})
	outcome, _ = page.Meaning.Get()
	require.Equal(t, api.MeaningOutcomeStatusReady, outcome.Status, "the same words, read once")
	understood, err := outcome.Understood.Get()
	require.NoError(t, err)
	assert.Equal(t, "recruitment", *understood.Family)
	require.Equal(t, []string{"Aina"}, names(page), "a Talent Acquisition Partner, though not a word matches; never the accountant")
	why, err := page.Items[0].Why.Get()
	require.NoError(t, err)
	assert.Equal(t, "Talent Acquisition Partner", *why.Title)
	assert.Equal(t, "now", *why.TitleWhen)
	assert.Equal(t, api.MeaningWhyStrengthStrong, why.Strength)
	_, more := readSearch(t, a, aiConn, tenant.ID, api.MeaningUnderstood{})
	assert.False(t, more, "not read again")

	// Work described, not a title: their CV says it.
	page = search(map[string]any{"meaning": "someone who made placements headhunting"})
	_, ok = readSearch(t, a, aiConn, tenant.ID, api.MeaningUnderstood{Describes: true, Family: str("recruitment")})
	require.True(t, ok)
	page = search(map[string]any{"meaning": "someone who made placements headhunting"})
	require.Contains(t, names(page), "Ben")
	for _, hit := range page.Items {
		if strings.HasPrefix(hit.Candidate.FullName, "Ben") {
			w, _ := hit.Why.Get()
			require.NotNil(t, w.Passage, "where their CV says it")
			assert.Contains(t, *w.Passage, "placements")
			assert.Contains(t, w.Words, "placements", "the search's own words found in their CV")
		}
	}

	// The other filters still hold.
	assert.Empty(t, names(search(map[string]any{"meaning": "recruiter", "location": "Penang"})))
	assert.Equal(t, []string{"Aina"}, names(search(map[string]any{"meaning": "recruiter", "location": "kuala"})))
	assert.Empty(t, names(search(map[string]any{"meaning": "recruiter", "q": "Chong"})))
}

// A CV that has just been read gets its candidate read by meaning at once,
// not at the next pass of the read-through.
func TestANewCVIsReadByMeaningAtOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "currentTitle": "Accountant"})
	a.request(http.MethodGet, "/v1/meaning/status", token, nil)
	require.Equal(t, 1, readByMeaning(t, a, aiConn, tenant.ID))

	doc := withCV(t, a, aiConn, token, c.Id, "Full set of accounts and bank reconciliations for twelve clients.")
	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc), api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: doc, Profile: nullable.NewNullNullable[api.ParsedProfile](), ErrorMessage: nullable.NewNullableWithValue("nothing found"),
	})
	job, ok := claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadCandidates)
	require.True(t, ok, "queued by the CV's reading, before any sweep")
	assert.Contains(t, string(job.Payload), "bank reconciliations", "with the new CV")
	assert.Contains(t, string(job.Payload), c.Id)
}

// Without the meaning model in the AI worker, a search by meaning says so
// (the screen offers the keyword search instead), and so does the status.
func TestWithoutTheMeaningModelASearchByMeaningSaysSo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	res := a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"meaning": "recruiter"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	job, ok := claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadQuery)
	require.True(t, ok)
	var asked api.MeaningReadQueryJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	yes := true
	answerAIJob(t, a, aiConn, job, api.JobKindMeaningQueryRead, api.MeaningQueryReadJob{
		QueryId: asked.QueryId, Model: "none", Unavailable: &yes, Needs: []api.MeaningNeedRead{},
	})
	var page api.CandidateSearchPage
	a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"meaning": "recruiter"}).JSON(&page)
	outcome, _ := page.Meaning.Get()
	assert.Equal(t, api.MeaningOutcomeStatusUnavailable, outcome.Status)
	var status api.MeaningStatus
	a.request(http.MethodGet, "/v1/meaning/status", token, nil).JSON(&status)
	assert.False(t, status.Available)
}

// A job's possible candidates include those whose work matches it though
// their CVs share none of its words, first, each with why; never someone
// already on the job.
func TestPossibleCandidatesForAJobAreFoundByMeaning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Talent Acquisition Lead",
		"description": "Own hiring for our engineering teams: sourcing, interviews and offers."})
	aina := a.createCandidate(token, map[string]any{"firstName": "Aina", "currentTitle": "Senior Recruiter"})
	ben := a.createCandidate(token, map[string]any{"firstName": "Ben", "currentTitle": "Accountant"})
	withCV(t, a, aiConn, token, ben.Id, "Bookkeeping, ledger reconciliations and audit support.")
	cara := a.createCandidate(token, map[string]any{"firstName": "Cara", "currentTitle": "Headhunter"})
	a.addToJob(token, job.Id, cara.Id)
	a.request(http.MethodGet, "/v1/meaning/status", token, nil)
	readByMeaning(t, a, aiConn, tenant.ID)

	suggest := func() api.MatchSuggestionList {
		res := a.request(http.MethodGet, "/v1/matches/suggestions?jobId="+job.Id, token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var list api.MatchSuggestionList
		res.JSON(&list)
		return list
	}
	list := suggest()
	outcome, err := list.Meaning.Get()
	require.NoError(t, err)
	assert.Equal(t, api.MeaningOutcomeStatusReading, outcome.Status, "the job is read first; meanwhile by its words")
	assert.Empty(t, list.Items, "no CV shares the job's words")
	asked, ok := readSearch(t, a, aiConn, tenant.ID, api.MeaningUnderstood{Role: str("talent acquisition lead"), Family: str("recruitment")})
	require.True(t, ok)
	require.NotNil(t, asked.Title)
	assert.Equal(t, "Talent Acquisition Lead", *asked.Title, "a job: its title")
	assert.Contains(t, asked.Body, "sourcing, interviews and offers", "and its description")
	assert.NotContains(t, asked.Body, "Maju", "never its employer")

	list = suggest()
	require.Len(t, list.Items, 1, "not the accountant, nor Cara who is on the job")
	assert.Equal(t, aina.Id, list.Items[0].CandidateId)
	why, err := list.Items[0].Why.Get()
	require.NoError(t, err)
	assert.Equal(t, "Senior Recruiter", *why.Title)
	assert.Empty(t, list.Items[0].Snippet, "found by meaning, not by the job's words")
}
