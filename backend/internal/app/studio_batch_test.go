package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Several candidates' CVs formatted at once with the same options: each
// one's newest CV they sent, as if started one by one; those that cannot be
// formatted now are skipped with the reason, the others go ahead.
func TestSeveralCVsAreFormattedAtOnceWithTheSameOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	off := newTestApp(t)
	a := newTestApp(t, matchingOn)
	candidate := func(name, file string, data []byte) string {
		c := a.createCandidate(token, map[string]any{"firstName": name})
		if file != "" {
			res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", file, data)
			require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		}
		return c.Id
	}
	siti := candidate("Siti", "cv.docx", minimalDOCX(t))
	lim := candidate("Lim", "cv.docx", minimalDOCX(t))
	nur := candidate("Nur", "", nil)
	wei := candidate("Wei", "cv.txt", []byte("Wei Ming\nAccountant at Contoh Berhad, 2019 to now\n"))
	stranger := testutil.Current.NewTenant(t, api.RoleRecruiter)
	theirs := a.createCandidate(stranger.Users[api.RoleRecruiter].Token, map[string]any{"firstName": "Elsewhere"}).Id

	batch := func(app *testApp, tok string, body map[string]any) *response {
		return app.request(http.MethodPost, "/v1/studio-cvs/batch", tok, body)
	}
	assert.Equal(t, http.StatusConflict, batch(off, token, map[string]any{"candidateIds": []string{siti}}).Status, "the AI is off")
	assert.Equal(t, http.StatusForbidden, batch(a, tenant.Users[api.RoleViewer].Token, map[string]any{"candidateIds": []string{siti}}).Status)
	many := make([]string, 21)
	for i := range many {
		many[i] = siti
	}
	assert.Equal(t, http.StatusBadRequest, batch(a, token, map[string]any{"candidateIds": many}).Status, "at most 20")
	res := batch(a, token, map[string]any{"candidateIds": []string{siti, lim}, "summary": "normal", "summaryDirection": "mention her expected salary"})
	assert.Equal(t, http.StatusBadRequest, res.Status, "a direction about pay stops it all")
	assert.Empty(t, a.studioCvs(token, siti), "nothing started")

	// Lim's CV is being formatted already.
	limCV := a.studioCvs(token, lim)
	require.Empty(t, limCV)
	var docs api.DocumentList
	a.request(http.MethodGet, "/v1/candidates/"+lim+"/documents", token, nil).JSON(&docs)
	res = a.request(http.MethodPost, "/v1/candidates/"+lim+"/studio-cvs", token, map[string]any{"documentId": docs.Items[0].Id})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)

	res = batch(a, token, map[string]any{
		"candidateIds": []string{siti, lim, nur, wei, theirs, siti},
		"blind":        true, "correctLanguage": true, "summary": "short", "summaryFocus": "leadership",
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var out api.StudioCvBatch
	res.JSON(&out)
	require.Len(t, out.Items, 5, "each candidate once, in the order given")
	got := map[string]api.StudioCvBatchItem{}
	for _, it := range out.Items {
		got[it.CandidateId] = it
	}
	assert.Equal(t, siti, out.Items[0].CandidateId)
	require.True(t, got[siti].Started)
	assert.True(t, got[siti].StudioCv.Blind)
	assert.False(t, got[lim].Started)
	assert.Contains(t, *got[lim].Reason, "being formatted already")
	assert.Contains(t, *got[nur].Reason, "no PDF or Word CV")
	assert.Contains(t, *got[wei].Reason, "no PDF or Word CV", "a text file is not formatted CV Studio's way")
	assert.Contains(t, *got[theirs].Reason, "not found")

	// The run is the same as one started on its own, with the options asked for.
	job := claimAIJobWhere(t, aiConn, "studioCvId", got[siti].StudioCv.Id)
	var p api.CvFormatStudioJob
	require.NoError(t, json.Unmarshal(job.Payload, &p))
	assert.True(t, p.Blind)
	assert.True(t, p.CorrectLanguage)
	assert.Equal(t, api.StudioCvSummaryShort, p.Summary)
	assert.Equal(t, api.StudioCvSummaryFocusLeadership, *p.SummaryFocus)
	answerAIJob(t, a, aiConn, job, api.JobKindCvStudioFormatted, studioResult(got[siti].StudioCv.Id, simpleDocx(t, "Senior Accountant"), ""))
	assert.Equal(t, api.StudioCvStatusDone, a.studioCvs(token, siti)[0].Status)
	assert.Contains(t, a.activityOf(token, "candidate", siti), "cv.studio_started", "audited as one started on its own")

	// Leave the AI queue tidy for other tests.
	limRun := a.studioCvs(token, lim)[0]
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "studioCvId", limRun.Id), api.JobKindCvStudioFormatted,
		studioResult(limRun.Id, simpleDocx(t, "Accountant"), ""))
}
