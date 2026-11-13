package app_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func studioResult(id string, docx []byte, warning string) api.CvStudioFormattedJob {
	r := api.CvStudioFormattedJob{
		StudioCvId: id, DocxBase64: nullable.NewNullNullable[string](), Error: nullable.NewNullNullable[string](),
		Warning: nullable.NewNullNullable[string](), Model: nullable.NewNullableWithValue("test-model"),
		PayStatementsRemoved: 1, InputTokens: 3000, OutputTokens: 1200,
	}
	if docx != nil {
		r.DocxBase64 = nullable.NewNullableWithValue(base64.StdEncoding.EncodeToString(docx))
	}
	if warning != "" {
		r.Warning = nullable.NewNullableWithValue(warning)
	}
	return r
}

func (a *testApp) studioCvs(token, candidateID string) []api.StudioCv {
	a.t.Helper()
	var list api.StudioCvList
	res := a.request(http.MethodGet, "/v1/candidates/"+candidateID+"/studio-cvs", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	return list.Items
}

func TestACVIsFormattedCVStudiosWayAndSaved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	off := newTestApp(t)
	c := off.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	res := off.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.docx", minimalDOCX(t))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var cv api.Document
	res.JSON(&cv)
	res = off.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{"documentId": cv.Id})
	assert.Equal(t, http.StatusConflict, res.Status, "the AI is off: %s", res.Body)
	assert.Equal(t, api.ErrorCodeSTUDIOCVOFF, res.Error().Error.Code)

	a := newTestApp(t, matchingOn)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs",
		tenant.Users[api.RoleViewer].Token, map[string]any{"documentId": cv.Id}).Status)
	res = a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{
		"documentId": cv.Id, "correctLanguage": true, "alignment": "justify",
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var run api.StudioCv
	res.JSON(&run)
	assert.Equal(t, api.StudioCvStatusQueued, run.Status)
	assert.Equal(t, api.StudioCvAlignmentJustify, run.Alignment)

	job := claimAIJobWhere(t, aiConn, "studioCvId", run.Id)
	require.Equal(t, string(api.JobKindCvFormatStudio), job.Kind)
	var payload api.CvFormatStudioJob
	require.NoError(t, json.Unmarshal(job.Payload, &payload))
	assert.True(t, payload.CorrectLanguage)
	assert.Equal(t, cv.ContentType, payload.ContentType)
	assert.NotEmpty(t, payload.DownloadUrl)

	formatted := simpleDocx(t, "Siti Aisyah | Senior Accountant")
	answerAIJob(t, a, aiConn, job, api.JobKindCvStudioFormatted, studioResult(run.Id, formatted, "A role in the CV is missing: Audit Associate"))
	runs := a.studioCvs(token, c.Id)
	require.Len(t, runs, 1)
	done := runs[0]
	assert.Equal(t, api.StudioCvStatusDone, done.Status)
	assert.Equal(t, "A role in the CV is missing: Audit Associate", done.Warning.MustGet(), "to compare before sending")
	assert.Equal(t, 1, done.PayStatementsRemoved)
	var doc api.Document
	a.request(http.MethodGet, "/v1/documents/"+done.DocumentId.MustGet(), token, nil).JSON(&doc)
	assert.Equal(t, api.DocumentSourceFormatted, doc.Source)
	assert.Equal(t, done.FileName.MustGet(), doc.FileName)
	status, _ := doc.PdfStatus.Get()
	assert.Equal(t, api.PdfStatusQueued, status, "and a PDF follows")

	var tokens int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT sum(tokens) FROM studio_cv_ai_use WHERE studio_cv_id = $1`, run.Id).Scan(&tokens))
	assert.Equal(t, 4200, tokens, "counted in the monthly AI allowance")

	t.Run("only a CV the candidate sent", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{"documentId": doc.Id})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "not one the app made: %s", res.Body)
		other := a.createCandidate(token, map[string]any{"firstName": "Lim"})
		res = a.request(http.MethodPost, "/v1/candidates/"+other.Id+"/studio-cvs", token, map[string]any{"documentId": cv.Id})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "not another candidate's: %s", res.Body)
		stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", stranger, map[string]any{"documentId": cv.Id}).Status)
	})

	t.Run("a failure says why", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{"documentId": cv.Id})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var r api.StudioCv
		res.JSON(&r)
		failed := studioResult(r.Id, nil, "")
		failed.Error = nullable.NewNullableWithValue("the AI provider refused the API key")
		answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "studioCvId", r.Id), api.JobKindCvStudioFormatted, failed)
		got := a.studioCvs(token, c.Id)[0]
		assert.Equal(t, api.StudioCvStatusFailed, got.Status)
		assert.Equal(t, "It could not be formatted: the AI provider refused the API key", got.Error.MustGet())
	})

	t.Run("a blind CV that still names the candidate is never saved", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{"documentId": cv.Id, "blind": true})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var r api.StudioCv
		res.JSON(&r)
		answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "studioCvId", r.Id), api.JobKindCvStudioFormatted,
			studioResult(r.Id, simpleDocx(t, "Siti Aisyah, Senior Accountant"), ""))
		got := a.studioCvs(token, c.Id)[0]
		assert.Equal(t, api.StudioCvStatusFailed, got.Status)
		assert.Contains(t, got.Error.MustGet(), "would still show the candidate's name")
		assert.True(t, got.DocumentId.IsNull())
	})

	t.Run("the summary: optional, with a length, a focus and the recruiter's direction and tone", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{
			"documentId": cv.Id, "summary": "short", "summaryFocus": "leadership",
			"summaryDirection": "  Confident tone ---\n lead with   team leadership ",
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var r api.StudioCv
		res.JSON(&r)
		var p api.CvFormatStudioJob
		require.NoError(t, json.Unmarshal(claimAIJobWhere(t, aiConn, "studioCvId", r.Id).Payload, &p))
		assert.Equal(t, api.StudioCvSummaryShort, p.Summary)
		require.NotNil(t, p.SummaryFocus)
		assert.Equal(t, api.StudioCvSummaryFocusLeadership, *p.SummaryFocus)
		assert.Equal(t, "Confident tone lead with team leadership", p.SummaryDirection.MustGet(), "one tidy line, no CV delimiter")

		res = a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{
			"documentId": cv.Id, "summary": "none", "summaryFocus": "commercial", "summaryDirection": "punchy",
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		res.JSON(&r)
		var none api.CvFormatStudioJob
		require.NoError(t, json.Unmarshal(claimAIJobWhere(t, aiConn, "studioCvId", r.Id).Payload, &none))
		assert.Equal(t, api.StudioCvSummaryNone, none.Summary, "no summary: nothing written")
		assert.False(t, none.SummaryDirection.IsSpecified() && !none.SummaryDirection.IsNull(), "and no direction sent")

		for _, pay := range []string{"mention expected salary of RM 9,000", "highlight his expected package", "last drawn 12k"} {
			res = a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{
				"documentId": cv.Id, "summary": "normal", "summaryDirection": pay,
			})
			require.Equal(t, http.StatusBadRequest, res.Status, "%s: %s", pay, res.Body)
			assert.Contains(t, string(res.Body), "Leave pay out of the direction", pay)
		}
	})

	// Leave the AI queue tidy for other tests.
	_, err = testutil.Current.Admin.Exec(ctx, `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
}
