package app_test

// Who a search by meaning compares: everyone the user may see, however many
// others are nearer (in their company or another on the server), a folder's
// or the filters' candidates only, and never anyone in the Bin.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// vectorText writes a vector as pgvector reads it.
func vectorText(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.FormatFloat(float64(x), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// readInBulk adds n candidates owned by ownerID, already read by meaning as
// holding title (as the stand-in reads it), straight into the database: a
// company larger than a test can make one by one. Their fingerprint is what
// the read-through finds, so they are not read again.
func readInBulk(t *testing.T, tenantID, ownerID, title string, n int) {
	t.Helper()
	vec := vectorText(fakeMeaning(title))
	fp := meaning.Fingerprint(meaning.Source{CurrentTitle: title})
	_, err := testutil.Current.Admin.Exec(context.Background(), `
		WITH c AS (
		  INSERT INTO candidate (tenant_id, first_name, current_title, created_by, owner_id)
		  SELECT $1, 'Bulk' || g, $2, $3, $3 FROM generate_series(1, $4::int) g RETURNING id
		), m AS (
		  INSERT INTO meaning_candidate (tenant_id, candidate_id, fingerprint, model, profile)
		  SELECT $1, id, $5, 'stand-in', $6::vector FROM c RETURNING candidate_id
		)
		INSERT INTO meaning_role (tenant_id, candidate_id, ordinal, title, current, field, work)
		SELECT $1, candidate_id, 0, $2, true, $6::vector, $6::vector FROM m`,
		tenantID, title, ownerID, n, fp, vec)
	require.NoError(t, err)
}

// startReading starts the company's read-through (a new company's starts
// with its first search or status check).
func startReading(t *testing.T, a *testApp, token string) {
	t.Helper()
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/meaning/status", token, nil).Status)
}

// asLargeCompany makes the company count as too large to compare everyone one
// by one (the indexes gather who to look at).
func asLargeCompany(t *testing.T, tenantID string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE meaning_sweep SET total_count = $2 WHERE tenant_id = $1`, tenantID, meaning.ExactMax*3)
	require.NoError(t, err)
}

// searchByMeaning runs a search by meaning, reading its words the first time.
func searchByMeaning(t *testing.T, a *testApp, aiConn *pgx.Conn, tenantID, token string, body map[string]any, understood api.MeaningUnderstood) api.CandidateSearchPage {
	t.Helper()
	run := func() api.CandidateSearchPage {
		res := a.request(http.MethodPost, "/v1/candidates/search", token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var page api.CandidateSearchPage
		res.JSON(&page)
		return page
	}
	page := run()
	if outcome, err := page.Meaning.Get(); err == nil && outcome.Status == api.MeaningOutcomeStatusReading {
		_, ok := readSearch(t, a, aiConn, tenantID, understood)
		require.True(t, ok)
		page = run()
	}
	outcome, err := page.Meaning.Get()
	require.NoError(t, err)
	require.Equal(t, api.MeaningOutcomeStatusReady, outcome.Status)
	return page
}

// A recruiter who may see only their own candidates finds them by meaning,
// though hundreds of others' candidates are nearer to the search (the
// database's indexes look at the nearest of everyone, then leave out those
// the user may not see).
func TestASearchByMeaningFindsEveryoneAUserMaySeeHoweverManyOthersAreNearer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	a.createCandidate(recruiter.Token, map[string]any{"firstName": "Aisyah", "currentTitle": "Talent Acquisition Partner"})
	a.createCandidate(recruiter.Token, map[string]any{"firstName": "Bala", "currentTitle": "Talent Acquisition Lead"})
	startReading(t, a, owner.Token)
	readByMeaning(t, a, aiConn, tenant.ID)
	readInBulk(t, tenant.ID, owner.ID, "Recruiter", 450) // the owner's: nearer to "recruiter" than either
	a.setRole(owner.Token, "recruiter", map[string]any{"version": 0, "permissions": authz.PermissionsForRole(api.RoleRecruiter),
		"candidateScope": "own", "jobCandidates": false}, http.StatusOK)

	understood := api.MeaningUnderstood{Role: str("recruiter"), Family: str("recruitment")}
	page := searchByMeaning(t, a, aiConn, tenant.ID, recruiter.Token, map[string]any{"meaning": "recruiter"}, understood)
	assert.ElementsMatch(t, []string{"Aisyah", "Bala"}, names(page), "their own, compared one by one")

	asLargeCompany(t, tenant.ID)
	page = searchByMeaning(t, a, aiConn, tenant.ID, recruiter.Token, map[string]any{"meaning": "recruiter"}, understood)
	assert.ElementsMatch(t, []string{"Aisyah", "Bala"}, names(page), "in a large company too: the indexes find too few, so those they may see are compared")

	all := searchByMeaning(t, a, aiConn, tenant.ID, owner.Token, map[string]any{"meaning": "recruiter", "limit": 100}, understood)
	assert.Len(t, all.Items, 100, "the owner sees everyone: the best 100")
	assert.True(t, strings.HasPrefix(names(all)[0], "Bulk"), "a Recruiter first")
}

// A search by meaning in a folder (or with filters) ranks the candidates in
// it, not the best of the whole company that happen to be in it.
func TestASearchByMeaningInAFolderRanksThoseInIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	aisyah := a.createCandidate(owner.Token, map[string]any{"firstName": "Aisyah", "currentTitle": "Talent Acquisition Partner"})
	bala := a.createCandidate(owner.Token, map[string]any{"firstName": "Bala", "currentTitle": "Talent Acquisition Lead"})
	a.createCandidate(owner.Token, map[string]any{"firstName": "Chong", "currentTitle": "Accountant"})
	startReading(t, a, owner.Token)
	readByMeaning(t, a, aiConn, tenant.ID)
	readInBulk(t, tenant.ID, recruiter.ID, "Recruiter", 450)
	asLargeCompany(t, tenant.ID)
	folder := a.folder(http.MethodPost, "/v1/folders", owner.Token, map[string]any{"kind": "candidate", "name": "Penang shortlist"}, http.StatusCreated)
	res := a.request(http.MethodPost, "/v1/folders/"+folder.Id+"/items", owner.Token, map[string]any{"itemIds": []string{aisyah.Id, bala.Id}})
	require.Less(t, res.Status, 300, "%s", res.Body)

	understood := api.MeaningUnderstood{Role: str("recruiter"), Family: str("recruitment")}
	page := searchByMeaning(t, a, aiConn, tenant.ID, owner.Token, map[string]any{"meaning": "recruiter", "folderId": folder.Id}, understood)
	assert.ElementsMatch(t, []string{"Aisyah", "Bala"}, names(page), "450 Recruiters are nearer, none in the folder")
	page = searchByMeaning(t, a, aiConn, tenant.ID, owner.Token, map[string]any{"meaning": "recruiter", "owner": "me"}, understood)
	assert.ElementsMatch(t, []string{"Aisyah", "Bala"}, names(page), "their own candidates, not the colleague's 450")
}

// Someone in the Bin is not found by meaning, nor counted as read; a CV in
// the Bin is no longer quoted or compared, and its candidate is read again.
func TestTheBinHidesWhatMeaningReadOfACandidateOrACV(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	a.createCandidate(token, map[string]any{"firstName": "Aisyah", "currentTitle": "Talent Acquisition Partner"})
	binned := a.createCandidate(token, map[string]any{"firstName": "Binned", "currentTitle": "Recruiter"})
	wrongCV := a.createCandidate(token, map[string]any{"firstName": "Chong", "currentTitle": "Accountant"})
	cv := withCV(t, a, aiConn, token, wrongCV.Id, "Headhunter. Made 40 placements a year, hiring and sourcing for MNC clients.")
	startReading(t, a, token)
	readByMeaning(t, a, aiConn, tenant.ID)

	visible := func(table string) int {
		var n int
		require.NoError(t, database.InTx(ctx, testutil.Current.App, authz.System(tenant.ID), func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
		}))
		return n
	}
	require.Equal(t, 3, visible("meaning_candidate"))
	res := a.request(http.MethodPost, "/v1/bin", token, map[string]any{"kind": "candidate", "id": binned.Id})
	require.Less(t, res.Status, 300, "%s", res.Body)
	assert.Equal(t, 2, visible("meaning_candidate"), "what was read of someone in the Bin is not seen")
	assert.Equal(t, 2, visible("meaning_role"))

	require.Equal(t, 1, visible("meaning_passage"))
	res = a.request(http.MethodPost, "/v1/bin", token, map[string]any{"kind": "document", "id": cv})
	require.Less(t, res.Status, 300, "%s", res.Body)
	assert.Zero(t, visible("meaning_passage"), "the CV in the Bin is no longer quoted, at once")
	job, ok := claimAIJobOfKind(t, aiConn, tenant.ID, api.JobKindMeaningReadCandidates)
	require.True(t, ok, "the candidate is read again, with no CV now")
	assert.Contains(t, string(job.Payload), wrongCV.Id)
	assert.NotContains(t, string(job.Payload), "Headhunter", fmt.Sprintf("not with the CV in the Bin: %s", cv))
}
