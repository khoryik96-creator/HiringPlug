package app_test

// Playing the Python worker's meaning model in Go tests: a stand-in that
// puts related job words on one axis (recruiter, talent acquisition and
// headhunter on one; accountant and bookkeeping on another), and places a
// title in the job family of the axis most of its work's words are on, so
// the tests can follow a search through the real pipeline. How well the
// real model reads is measured in ai/tests/test_meaning_bench.py.

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var meaningAxes = map[string]int{}

// standInFamilies are the stand-in's job families, one per axis.
var standInFamilies = []string{"recruitment", "accounting", "sales", "software", "finance management"}

func init() {
	for axis, words := range [][]string{
		{"recruiter", "recruitment", "recruiting", "talent", "acquisition", "headhunter", "headhunting", "hiring", "hired", "placements", "placed", "sourcing"},
		{"accountant", "accounts", "accounting", "bookkeeping", "ledger", "reconciliations", "audit", "auditors"},
		{"sales", "selling", "sold", "business", "development", "quota", "clients"},
		{"software", "developer", "engineer", "programmer", "code", "golang", "react"},
		{"budgets", "budgeting", "forecasts", "forecasting"},
	} {
		for _, w := range words {
			meaningAxes[w] = axis
		}
	}
}

var meaningWord = regexp.MustCompile(`[a-z0-9]+`)

// fakeMeaning reads a text into 384 numbers: shared axes for related words, others spread thinly.
func fakeMeaning(text string) []float32 {
	v := make([]float64, 384)
	for _, w := range meaningWord.FindAllString(strings.ToLower(text), -1) {
		if axis, ok := meaningAxes[w]; ok {
			v[axis]++
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[10+int(h.Sum32()%370)] += 0.3
	}
	norm := 0.0
	for _, x := range v {
		norm += x * x
	}
	out := make([]float32, 384)
	if norm == 0 {
		out[383] = 1
		return out
	}
	for i, x := range v {
		out[i] = float32(x / math.Sqrt(norm))
	}
	return out
}

// standInFamily places work in the family of the axis most of its words are
// on (none: no such words, or two axes as many).
func standInFamily(text string) *string {
	counts := make([]int, len(standInFamilies))
	for _, w := range meaningWord.FindAllString(strings.ToLower(text), -1) {
		if axis, ok := meaningAxes[w]; ok {
			counts[axis]++
		}
	}
	best, tie := -1, false
	for axis, n := range counts {
		switch {
		case n == 0:
		case best < 0 || n > counts[best]:
			best, tie = axis, false
		case n == counts[best]:
			tie = true
		}
	}
	if best < 0 || tie {
		return nil
	}
	return &standInFamilies[best]
}

// claimAIJobOfKind claims the tenant's next AI job of a kind (none: ok false).
func claimAIJobOfKind(t *testing.T, conn *pgx.Conn, tenantID string, kind api.JobKind) (claimedJob, bool) {
	t.Helper()
	claimLock.Lock()
	defer claimLock.Unlock()
	ctx := context.Background()
	tag, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() - interval '1 hour', priority = 1
		WHERE queue = 'ai' AND status = 'pending' AND kind = $1 AND tenant_id = $2`, string(kind), tenantID)
	require.NoError(t, err)
	if tag.RowsAffected() == 0 {
		return claimedJob{}, false
	}
	for range 20 {
		var j claimedJob
		var reqID *string
		err = conn.QueryRow(ctx, `SELECT id, tenant_id, kind, payload, request_id FROM job_claim_kinds('ai', 'python-test', 60, $1)`,
			[]string{string(kind)}).Scan(&j.ID, &j.TenantID, &j.Kind, &j.Payload, &reqID)
		require.NoError(t, err)
		if j.TenantID == tenantID {
			if reqID != nil {
				j.RequestID = *reqID
			}
			return j, true
		}
		// Another test's job of the same kind: put it back.
		_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET status = 'pending', attempts = greatest(attempts - 1, 0),
			locked_by = NULL, locked_until = NULL, run_after = now(), priority = 9 WHERE id = $1`, j.ID)
		require.NoError(t, err)
	}
	t.Fatal("could not claim this tenant's meaning job")
	return claimedJob{}, false
}

// readByMeaning plays the worker: it reads every candidate queued for the
// tenant with the stand-in model, and saves what it read (the Go worker).
func readByMeaning(t *testing.T, a *testApp, conn *pgx.Conn, tenantID string) int {
	t.Helper()
	runCoreJobsUntilIdle(t, a, tenantID) // the read-through queues them
	read := 0
	for {
		job, ok := claimAIJobOfKind(t, conn, tenantID, api.JobKindMeaningReadCandidates)
		if !ok {
			return read
		}
		var asked api.MeaningReadCandidatesJob
		require.NoError(t, json.Unmarshal(job.Payload, &asked))
		out := api.MeaningCandidatesReadJob{Model: "stand-in", Candidates: []api.MeaningCandidateRead{}}
		for _, c := range asked.Candidates {
			r := api.MeaningCandidateRead{CandidateId: c.CandidateId, Fingerprint: c.Fingerprint, Roles: []api.MeaningRoleRead{}, Passages: []api.MeaningPassageRead{}}
			var titles []string
			for k, role := range c.Roles {
				work := role.Title
				if role.Description != nil {
					work += ". " + *role.Description
				}
				placedBy := work
				if k == 0 && role.Description == nil && c.Cv != nil {
					placedBy += ". " + c.Cv.Text // the newest title, placed by the CV's lines as the worker does
				}
				r.Roles = append(r.Roles, api.MeaningRoleRead{
					Ordinal: role.Ordinal, Title: role.Title, EndYear: role.EndYear, Field: fakeMeaning(role.Title), Work: fakeMeaning(work),
					Family: standInFamily(placedBy),
				})
				titles = append(titles, role.Title)
			}
			profile := strings.Join(titles, ". ")
			if c.Cv != nil {
				r.Passages = append(r.Passages, api.MeaningPassageRead{Ordinal: 0, DocumentId: c.Cv.DocumentId, Passage: c.Cv.Text, Vec: fakeMeaning(c.Cv.Text)})
				profile += ". " + c.Cv.Text
			}
			if profile != "" {
				v := fakeMeaning(profile)
				r.Profile = &v
			}
			out.Candidates = append(out.Candidates, r)
			read++
		}
		answerAIJob(t, a, conn, job, api.JobKindMeaningCandidatesRead, out)
	}
}

// readSearch plays the worker on the tenant's waiting search (or job): ok false when none waits.
func readSearch(t *testing.T, a *testApp, conn *pgx.Conn, tenantID string, understood api.MeaningUnderstood) (api.MeaningReadQueryJob, bool) {
	t.Helper()
	job, ok := claimAIJobOfKind(t, conn, tenantID, api.JobKindMeaningReadQuery)
	if !ok {
		return api.MeaningReadQueryJob{}, false
	}
	var asked api.MeaningReadQueryJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	text := asked.Body
	if asked.Title != nil {
		text = *asked.Title
	}
	role, whole := fakeMeaning(text), fakeMeaning(asked.Body)
	answerAIJob(t, a, conn, job, api.JobKindMeaningQueryRead, api.MeaningQueryReadJob{
		QueryId: asked.QueryId, Model: "stand-in", Role: &role, Whole: &whole, Understood: &understood, Needs: []api.MeaningNeedRead{},
		Describes: understood.Describes,
	})
	return asked, true
}
