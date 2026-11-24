package app_test

// Every AI feature records what it used in the AI use ledger (aiusage): the
// use stays counted when its record is deleted, and a result delivered twice
// counts once.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var bigAllowance = map[string]string{"HP_AI_PROVIDER": "openai_compatible", "HP_AI_MONTHLY_TOKENS": "1000000"}

// usedThisMonth is the company's AI use this month, as the assistant shows it.
func (a *testApp) usedThisMonth(token string) int64 {
	a.t.Helper()
	var status api.AssistantStatus
	res := a.request(http.MethodGet, "/v1/assistant", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&status)
	return status.UsedTokens
}

// ledger is the company's AI use rows: feature -> tokens, and who asked.
func ledger(t *testing.T, tenantID string) map[string]int64 {
	t.Helper()
	rows, err := testutil.Current.Admin.Query(context.Background(),
		`SELECT feature, sum(input_tokens + output_tokens)::bigint FROM ai_use WHERE tenant_id = $1 GROUP BY feature`, tenantID)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var f string
		var n int64
		require.NoError(t, rows.Scan(&f, &n))
		out[f] = n
	}
	require.NoError(t, rows.Err())
	return out
}

func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

func TestSpentAITokensStayCountedWhenTheirRecordsAreDeleted(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner]
	token := owner.Token

	company := a.createCompany(token, map[string]any{"name": "Contoh Logistik Sdn Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Hub Manager",
		"description": "Run the Shah Alam hub: 40 staff, two shifts, same-day dispatch."})

	// The assistant's summary of the job (forgotten when the job is deleted).
	res := a.ask(token, map[string]any{"subjectType": "job", "subjectId": job.Id, "kind": "summary"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var q api.AssistantAnswer
	res.JSON(&q)
	aiAnswers(t, a, aiConn, q.Id, api.AssistantAnsweredJob{
		Answer: nullable.NewNullableWithValue("A busy hub."), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullNullable[string](), InputTokens: 1000, OutputTokens: 200,
	})

	// A blind rewrite of the job's ad (an ad's rewrite goes with the job).
	ad := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{"title": "Hub Manager", "description": longAd}, http.StatusCreated)
	var rewrites api.BlindRewriteList
	a.request(http.MethodGet, "/v1/job-ads/"+ad.Id+"/blind-rewrites", token, nil).JSON(&rewrites)
	require.Len(t, rewrites.Items, 1, "a new ad is rewritten on its own")
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "rewriteId", rewrites.Items[0].Id), api.JobKindJdBlinded,
		rewriteAnswer(rewrites.Items[0].Id, nil, "the AI gave up")) // 1,600 tokens, used all the same

	// Interview prep for the job (goes with the job).
	a.interviewQuestion(token, job.Id, map[string]any{"question": "How would you plan a peak week?", "kind": "situational"})
	res = a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/interview-preps", token, map[string]any{})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var prep api.InterviewPrep
	res.JSON(&prep)
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "prepId", prep.Id), api.JobKindInterviewPrepWritten,
		prepWritten(prep.Id, nil, "could not")) // 4,500 tokens

	// A job description the AI read (deleted on its own).
	res = a.upload("/v1/jd-files", token, "", "JD.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var jd api.JdFile
	res.JSON(&jd)
	tokens := 900
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "jdFileId", jd.Id), api.JobKindJobJdRead, api.JobJdReadJob{
		JdFileId: jd.Id, Status: api.ExtractionStatusExtracted, Text: nullable.NewNullableWithValue("Hub Manager\nRun the hub."),
		ErrorMessage: nullable.NewNullNullable[string](), AiTokens: &tokens, AiDetails: nullable.NewNullNullable[api.JdAiDetails](),
	})

	const spent = 1200 + 1600 + 4500 + 900
	require.EqualValues(t, spent, a.usedThisMonth(token))
	assert.Equal(t, map[string]int64{"assistant": 1200, "jd_blind": 1600, "interview_prep": 4500, "jd_reading": 900}, ledger(t, tenant.ID))
	var askedBy string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT user_id FROM ai_use WHERE tenant_id = $1 AND feature = 'jd_reading'`, tenant.ID).Scan(&askedBy))
	assert.Equal(t, owner.ID, askedBy, "who uploaded the job description")

	require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/jd-files/"+jd.Id, token, nil).Status)
	assert.EqualValues(t, spent, a.usedThisMonth(token), "a deleted job description's reading stays counted")

	res = a.bin(token, map[string]any{"kind": "company", "id": company.Id}) // its job goes with it
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	purgeNow(t, a, tenant.ID)
	assert.Zero(t, countRows(t, `SELECT count(*) FROM job_order WHERE tenant_id = $1`, tenant.ID), "the job is deleted for good")
	assert.Zero(t, countRows(t, `SELECT count(*) FROM assistant_answer WHERE tenant_id = $1`, tenant.ID), "its answers are forgotten")
	assert.Zero(t, countRows(t, `SELECT count(*) FROM interview_prep WHERE tenant_id = $1`, tenant.ID))
	assert.Zero(t, countRows(t, `SELECT count(*) FROM job_ad_blind_rewrite WHERE tenant_id = $1`, tenant.ID))
	assert.EqualValues(t, spent, a.usedThisMonth(token), "what the AI used for a purged job stays counted")
}

func TestARepeatedAIResultCountsOnce(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance)
	aiConn := aiConnFor(t)

	t.Run("a CV read", func(t *testing.T) {
		tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
		token := tenant.Users[api.RoleRecruiter].Token
		doc, job, payload := readCV(t, a, aiConn, token, "SITI AISYAH works at Contoh Sdn Bhd")
		require.NotNil(t, payload.RunKey, "the reading's key goes with it")
		tokens := 1700
		result := api.DocumentCvParsedJob{
			DocumentId: doc.Id, ErrorMessage: nullable.NewNullNullable[string](), AiTokens: &tokens, RunKey: payload.RunKey,
			Profile: nullable.NewNullableWithValue(api.ParsedProfile{
				Parser: "rules@1+ai", FullName: sourced("Siti Aisyah", "SITI AISYAH"),
				Skills: []api.SourcedText{}, Experience: []api.ParsedJob{}, Education: []api.ParsedEducation{},
			}),
		}
		answerAIJob(t, a, aiConn, job, api.JobKindDocumentCvParsed, result)
		answerAIJob(t, a, aiConn, job, api.JobKindDocumentCvParsed, result) // delivered again
		assert.EqualValues(t, 1700, a.usedThisMonth(token))
		assert.Equal(t, 1, countRows(t, `SELECT count(*) FROM ai_use WHERE tenant_id = $1 AND feature = 'cv_reading'`, tenant.ID))
	})

	t.Run("a CV Studio run", func(t *testing.T) {
		tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
		token := tenant.Users[api.RoleRecruiter].Token
		c := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.docx", minimalDOCX(t))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var cv api.Document
		res.JSON(&cv)
		res = a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{"documentId": cv.Id})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var run api.StudioCv
		res.JSON(&run)
		job := claimAIJobWhere(t, aiConn, "studioCvId", run.Id)
		failed := studioResult(run.Id, nil, "") // 4,200 tokens, used even though it failed
		failed.Error = nullable.NewNullableWithValue("the AI gave up")
		answerAIJob(t, a, aiConn, job, api.JobKindCvStudioFormatted, failed)
		answerAIJob(t, a, aiConn, job, api.JobKindCvStudioFormatted, failed) // delivered again
		assert.EqualValues(t, 4200, a.usedThisMonth(token))
		var who string
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT user_id FROM ai_use WHERE tenant_id = $1 AND feature = 'cv_studio'`, tenant.ID).Scan(&who))
		assert.Equal(t, tenant.Users[api.RoleRecruiter].ID, who)
	})

	t.Run("interview prep with bad counts", func(t *testing.T) {
		tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
		token := tenant.Users[api.RoleRecruiter].Token
		company := a.createCompany(token, map[string]any{"name": "Maju Bank Berhad"})
		job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Data Engineer", "description": "Run the data platform."})
		a.interviewQuestion(token, job.Id, map[string]any{"question": "How do you test a pipeline?", "kind": "technical"})
		res := a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/interview-preps", token, map[string]any{})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var prep api.InterviewPrep
		res.JSON(&prep)
		claimed := claimAIJobWhere(t, aiConn, "prepId", prep.Id)
		bad := prepWritten(prep.Id, nil, "could not")
		bad.InputTokens, bad.OutputTokens = -5000, 700 // never below 0: a bad result cannot lower the month's use
		answerAIJob(t, a, aiConn, claimed, api.JobKindInterviewPrepWritten, bad)
		assert.EqualValues(t, 700, a.usedThisMonth(token))
		assert.Equal(t, 700, countRows(t, `SELECT tokens FROM interview_prep WHERE id = $1`, prep.Id), "saved within 0..1<<30")
	})
}

func TestAKeyCheckCountsInTheCompanysAIUse(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_KEYS_SECRET": aiKeysSecret})
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner]
	res := a.request(http.MethodPut, "/v1/ai-providers/deepseek", owner.Token, map[string]any{"model": "test-model", "apiKey": "sk-test-0000-1111"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	job := claimCheck(t, aiConn, savedSealed(t, tenant.ID, "deepseek"))
	var check api.AiProviderCheckJob
	require.NoError(t, json.Unmarshal(job.Payload, &check))
	require.NotNil(t, check.CheckId, "each check has its own ID")
	require.NotNil(t, check.AskedBy)
	assert.Equal(t, owner.ID, *check.AskedBy)

	in, out := int64(21), int64(2)
	result := api.AiProviderCheckedJob{Provider: check.Provider, SealedKey: check.Access.SealedKey, Ok: true,
		Message: nullable.NewNullNullable[string](), CheckId: check.CheckId, AskedBy: check.AskedBy, InputTokens: &in, OutputTokens: &out}
	answerAIJob(t, a, aiConn, job, api.JobKindAiproviderChecked, result)
	answerAIJob(t, a, aiConn, job, api.JobKindAiproviderChecked, result) // delivered again
	assert.Equal(t, map[string]int64{"provider_check": 23}, ledger(t, tenant.ID), "counted once")
	var who string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT user_id FROM ai_use WHERE tenant_id = $1 AND source_id = $2`, tenant.ID, *check.CheckId).Scan(&who))
	assert.Equal(t, owner.ID, who, "who asked for the check")
}

// A CV read again while its first reading is still with the AI is two AI
// runs: both count, though only the first result to land is kept.
func TestACVReadAgainWhileBeingReadCountsBothReadings(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	doc, first, firstPayload := readCV(t, a, aiConn, token, "SITI AISYAH works at Contoh Sdn Bhd")

	// Read again before the first reading's result lands.
	res := a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, "SITI AISYAH works at Contoh Sdn Bhd"))
	second := claimAIJobFor(t, aiConn, doc.Id)
	require.Equal(t, "document.parse_cv", second.Kind)
	var secondPayload api.DocumentParseCvJob
	require.NoError(t, json.Unmarshal(second.Payload, &secondPayload))
	require.NotEqual(t, *firstPayload.RunKey, *secondPayload.RunKey, "each reading has its own key")

	reading := func(runKey *string, tokens int) api.DocumentCvParsedJob {
		return api.DocumentCvParsedJob{
			DocumentId: doc.Id, ErrorMessage: nullable.NewNullNullable[string](), AiTokens: &tokens, RunKey: runKey,
			Profile: nullable.NewNullableWithValue(api.ParsedProfile{
				Parser: "rules@1+ai", FullName: sourced("Siti Aisyah", "SITI AISYAH"),
				Skills: []api.SourcedText{}, Experience: []api.ParsedJob{}, Education: []api.ParsedEducation{},
			}),
		}
	}
	answerAIJob(t, a, aiConn, first, api.JobKindDocumentCvParsed, reading(firstPayload.RunKey, 1700))
	answerAIJob(t, a, aiConn, second, api.JobKindDocumentCvParsed, reading(secondPayload.RunKey, 1300))
	answerAIJob(t, a, aiConn, second, api.JobKindDocumentCvParsed, reading(secondPayload.RunKey, 1300)) // delivered again
	assert.EqualValues(t, 1700+1300, a.usedThisMonth(token), "both readings used the AI")
	assert.Equal(t, 2, countRows(t, `SELECT count(*) FROM ai_use WHERE tenant_id = $1 AND feature = 'cv_reading'`, tenant.ID))
}

// A job description uploaded last month and read by the AI this month
// counts in this month's use: when the AI read it, not when it came.
func TestAJobDescriptionReadCountsWhenTheAIReadIt(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	res := a.upload("/v1/jd-files", token, "", "JD.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var jd api.JdFile
	res.JSON(&jd)
	lastMonth := agencyMonth(t).AddDate(0, 0, -3)
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE job_description_file SET created_at = $1 WHERE id = $2`, lastMonth, jd.Id)
	require.NoError(t, err)

	tokens := 900
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "jdFileId", jd.Id), api.JobKindJobJdRead, api.JobJdReadJob{
		JdFileId: jd.Id, Status: api.ExtractionStatusExtracted, Text: nullable.NewNullableWithValue("Hub Manager\nRun the hub."),
		ErrorMessage: nullable.NewNullNullable[string](), AiTokens: &tokens, AiDetails: nullable.NewNullNullable[api.JdAiDetails](),
	})
	assert.EqualValues(t, 900, a.usedThisMonth(token), "read this month")
	assert.Equal(t, 1, countRows(t, `SELECT count(*) FROM ai_use WHERE tenant_id = $1 AND feature = 'jd_reading' AND used_at >= $2`,
		tenant.ID, agencyMonth(t)))
}
