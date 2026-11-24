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
	dina := a.createCandidate(token, map[string]any{"firstName": "Dina", "currentTitle": "Procurement Manager"})
	withCV(t, a, aiConn, token, dina.Id, "Interviews and offers with suppliers; tenders and contracts.") // the job's words, not its work
	fiona := a.createCandidate(token, map[string]any{"firstName": "Fiona"})                              // no title: meaning cannot say her field
	withCV(t, a, aiConn, token, fiona.Id, "Interviews and offers for our engineering teams.")
	a.request(http.MethodGet, "/v1/meaning/status", token, nil)
	readByMeaning(t, a, aiConn, tenant.ID)
	eve := a.createCandidate(token, map[string]any{"firstName": "Eve"})
	withCV(t, a, aiConn, token, eve.Id, "Interviews and offers for engineering teams.") // not read by meaning yet

	suggest := func() api.MatchSuggestionList {
		res := a.request(http.MethodGet, "/v1/matches/suggestions?jobId="+job.Id, token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var list api.MatchSuggestionList
		res.JSON(&list)
		return list
	}
	ids := func(list api.MatchSuggestionList) []string {
		out := []string{}
		for _, s := range list.Items {
			out = append(out, s.CandidateId)
		}
		return out
	}
	list := suggest()
	outcome, err := list.Meaning.Get()
	require.NoError(t, err)
	assert.Equal(t, api.MeaningOutcomeStatusReading, outcome.Status, "the job is read first; meanwhile by its words")
	assert.ElementsMatch(t, []string{dina.Id, eve.Id, fiona.Id}, ids(list), "those whose CVs share the job's words")
	asked, ok := readSearch(t, a, aiConn, tenant.ID, api.MeaningUnderstood{Role: str("talent acquisition lead"), Family: str("recruitment")})
	require.True(t, ok)
	require.NotNil(t, asked.Title)
	assert.Equal(t, "Talent Acquisition Lead", *asked.Title, "a job: its title")
	assert.Contains(t, asked.Body, "sourcing, interviews and offers", "and its description")
	assert.NotContains(t, asked.Body, "Maju", "never its employer")

	list = suggest()
	require.Len(t, list.Items, 3)
	assert.Equal(t, aina.Id, list.Items[0].CandidateId, "by meaning first")
	assert.ElementsMatch(t, []string{eve.Id, fiona.Id}, ids(list)[1:],
		"then by the job's words: Eve, not yet read, and Fiona, whose field meaning cannot tell (no title); "+
			"not Dina, whom meaning read by her title and turned down; never the accountant, nor Cara who is on the job")
	assert.False(t, list.Items[1].Why.IsSpecified() && !list.Items[1].Why.IsNull(), "by the job's words only")
	why, err := list.Items[0].Why.Get()
	require.NoError(t, err)
	assert.Equal(t, "Senior Recruiter", *why.Title)
	assert.Empty(t, list.Items[0].Snippet, "found by meaning, not by the job's words")
}

// A new meaning model (or new reading rules, which the worker names as a new
// model) has everyone read again at once: vectors of different models are
// never compared.
func TestANewMeaningModelReadsEveryoneAgain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Aina", "currentTitle": "Recruiter"})
	a.request(http.MethodGet, "/v1/meaning/status", token, nil)
	require.Equal(t, 1, readByMeaning(t, a, aiConn, tenant.ID))
	runCoreJobsUntilIdle(t, a, tenant.ID)
	_, again := claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadCandidates)
	require.False(t, again, "read once")

	a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"meaning": "recruiter"})
	job, ok := claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadQuery)
	require.True(t, ok)
	var asked api.MeaningReadQueryJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	// The read-through is part-way through a pass when the new model comes.
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE meaning_sweep SET cursor_id = $2 WHERE tenant_id = $1`, tenant.ID, c.Id)
	require.NoError(t, err)
	v := fakeMeaning("recruiter")
	answerAIJob(t, a, aiConn, job, api.JobKindMeaningQueryRead, api.MeaningQueryReadJob{
		QueryId: asked.QueryId, Model: "stand-in, rules 2", Role: &v, Whole: &v, Needs: []api.MeaningNeedRead{},
	})
	runCoreJobsUntilIdle(t, a, tenant.ID) // the read-through starts again at once, from the beginning
	job, ok = claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadCandidates)
	require.True(t, ok, "everyone is read again with the new model, those before where the pass had got to too")
	assert.Contains(t, string(job.Payload), c.Id)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET status = 'pending', locked_by = NULL, locked_until = NULL WHERE id = $1`, job.ID)
	require.NoError(t, err) // put back: read below

	search := func() api.CandidateSearchPage {
		var page api.CandidateSearchPage
		a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"meaning": "recruiter"}).JSON(&page)
		return page
	}
	outcome, err := search().Meaning.Get()
	require.NoError(t, err)
	assert.Equal(t, api.MeaningOutcomeStatusReading, outcome.Status, "and the search: its words are read again by the new rules")
	job, ok = claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadQuery)
	require.True(t, ok)
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	answerAIJob(t, a, aiConn, job, api.JobKindMeaningQueryRead, api.MeaningQueryReadJob{
		QueryId: asked.QueryId, Model: "stand-in, rules 2", Role: &v, Whole: &v, Needs: []api.MeaningNeedRead{},
	})
	assert.Empty(t, names(search()), "Aina's reading by the other model is not compared with the new one")
	require.Equal(t, 1, readByMeaningWith(t, a, aiConn, tenant.ID, "stand-in, rules 2"))
	assert.Equal(t, []string{"Aina"}, names(search()), "once read again, she is found")
}

// A recruiter does not take another field for the one asked for, however
// close the words: of three Executives, the one keeping the books comes
// first for an accounts search, the one doing budgets (a neighbouring
// field) close behind, and the one selling counts for less.
func TestAnotherFieldCountsForLessInASearchByMeaning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	// Added in a fixed order, not a map's random one: Mei and Siti score the same, so the search's
	// order between them must not depend on chance.
	for _, c := range []struct{ name, cv string }{
		{"Mei", "Executive. Accounts, ledger and bookkeeping."},
		{"Siti", "Executive. Budgets, forecasts and forecasting."},
		{"Kumar", "Executive. Sales: selling software, new business and quota."},
	} {
		made := a.createCandidate(token, map[string]any{"firstName": c.name, "currentTitle": "Executive"})
		withCV(t, a, aiConn, token, made.Id, c.cv)
	}
	a.request(http.MethodGet, "/v1/meaning/status", token, nil)
	require.Equal(t, 3, readByMeaning(t, a, aiConn, tenant.ID))

	search := func() api.CandidateSearchPage {
		var page api.CandidateSearchPage
		res := a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"meaning": "executive"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&page)
		return page
	}
	search()
	_, ok := readSearch(t, a, aiConn, tenant.ID, api.MeaningUnderstood{
		Role: str("executive"), Family: str("accounting"), Near: &[]string{"audit", "finance management"},
	})
	require.True(t, ok)
	page := search()
	require.Equal(t, []string{"Mei", "Siti", "Kumar"}, names(page))
	whys := map[string]api.MeaningWhy{}
	for _, hit := range page.Items {
		w, err := hit.Why.Get()
		require.NoError(t, err)
		whys[strings.Fields(hit.Candidate.FullName)[0]] = w
	}
	assert.Equal(t, "sales", *whys["Kumar"].Family, "why: their title's field")
	assert.Equal(t, api.MeaningWhyStrengthStrong, whys["Mei"].Strength)
	assert.Equal(t, api.MeaningWhyStrengthStrong, whys["Siti"].Strength, "a neighbouring field counts in full")
	assert.NotEqual(t, api.MeaningWhyStrengthStrong, whys["Kumar"].Strength, "the same title in another field")
	assert.Less(t, whys["Kumar"].Score, float32(0.8)*whys["Siti"].Score)
}
