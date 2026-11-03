package app_test

import (
	"context"
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

var assistantOn = map[string]string{"HP_AI_PROVIDER": "openai_compatible", "HP_AI_MONTHLY_TOKENS": "5000"}

func (a *testApp) ask(token string, body map[string]any) *response {
	a.t.Helper()
	return a.request(http.MethodPost, "/v1/assistant/answers", token, body)
}

func (a *testApp) answers(token, subjectType, subjectID string) []api.AssistantAnswer {
	a.t.Helper()
	var list api.AssistantAnswerList
	res := a.request(http.MethodGet, "/v1/assistant/answers?subjectType="+subjectType+"&subjectId="+subjectID, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	return list.Items
}

// aiAnswers plays the Python worker: it claims the question and answers it.
func aiAnswers(t *testing.T, a *testApp, conn *pgx.Conn, answerID string, answer api.AssistantAnsweredJob) api.AssistantAskJob {
	t.Helper()
	job := claimAIJobWhere(t, conn, "answerId", answerID)
	require.Equal(t, string(api.JobKindAssistantAsk), job.Kind)
	var asked api.AssistantAskJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	answer.AnswerId = answerID
	answerAIJob(t, a, conn, job, api.JobKindAssistantAnswered, answer)
	return asked
}

func TestTheAssistantSeesOnlyWhatItMayAndNoSalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	c := a.createCandidate(token, map[string]any{
		"firstName": "Siti", "lastName": "Aisyah", "email": "siti.aisyah@mail.com", "phone": "+60 12-345 6789", "currentTitle": "Senior Accountant",
	})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", token, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8765", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc.Id), api.JobKindDocumentTextExtracted,
		extractedText(doc.Id, "SITI AISYAH\nsiti.aisyah@mail.com 012-345 6789\nAccountant at Contoh Sdn Bhd, 2019-2023. Salary RM 8,765."))
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance Manager", "description": "Lead the finance team.\nSalary: RM 9,999 a month"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/jobs/"+job.Id+"/salary", token, map[string]any{
		"expectedVersion": 0, "range": map[string]any{"min": "13579", "currency": "MYR", "period": "month"},
	}).Status)
	appl := a.addToJob(token, job.Id, c.Id)

	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.Equal(t, api.AssistantStatus{Enabled: true, UsedTokens: 0, MonthlyTokens: 5000}, status)

	res = a.ask(token, map[string]any{"subjectType": "application", "subjectId": appl.Id, "kind": "fit"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var queued api.AssistantAnswer
	res.JSON(&queued)
	assert.Equal(t, api.AssistantAnswerStatusQueued, queued.Status)

	asked := aiAnswers(t, a, aiConn, queued.Id, api.AssistantAnsweredJob{
		Answer: nullable.NewNullableWithValue("A good fit: six years of accounting."), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullableWithValue("deepseek-chat"), InputTokens: 1200, OutputTokens: 300,
	})
	sent, err := json.Marshal(asked.Sections)
	require.NoError(t, err)
	for _, leak := range []string{"Siti", "SITI", "Aisyah", "siti.aisyah@mail.com", "345 6789", "13579", "13,579", "8765", "8,765", "9,999"} {
		assert.NotContains(t, string(sent), leak, "the AI never sees contact details, the name or salary")
	}
	assert.Contains(t, string(sent), "Finance Manager")
	assert.Contains(t, string(sent), "Lead the finance team.")
	assert.Contains(t, string(sent), "Accountant at Contoh Sdn Bhd, 2019-2023", "the CV's text, scrubbed")
	assert.Equal(t, api.AssistantKindFit, asked.Kind)

	got := a.answers(token, "application", appl.Id)
	require.Len(t, got, 1)
	assert.Equal(t, api.AssistantAnswerStatusDone, got[0].Status)
	assert.Equal(t, "A good fit: six years of accounting.", got[0].Answer.MustGet())
	assert.Equal(t, "deepseek-chat", got[0].Model.MustGet())
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.EqualValues(t, 1500, status.UsedTokens)

	t.Run("answers are only for the person who asked", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Empty(t, a.answers(other, "application", appl.Id))
		res := a.ask(other, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
		assert.Equal(t, http.StatusNotFound, res.Status, "another company's candidate")
		assert.Equal(t, http.StatusForbidden, a.ask(tenant.Users[api.RoleViewer].Token,
			map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"}).Status)
	})

	t.Run("a failed answer says why", func(t *testing.T) {
		res := a.ask(token, map[string]any{"subjectType": "job", "subjectId": job.Id, "kind": "question", "question": "What should I ask?"})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var q api.AssistantAnswer
		res.JSON(&q)
		aiAnswers(t, a, aiConn, q.Id, api.AssistantAnsweredJob{
			Answer: nullable.NewNullNullable[string](), Error: nullable.NewNullableWithValue("the AI provider did not answer in time"),
			Model: nullable.NewNullNullable[string](),
		})
		got := a.answers(token, "job", job.Id)
		require.Len(t, got, 1)
		assert.Equal(t, api.AssistantAnswerStatusFailed, got[0].Status)
		assert.Equal(t, "the AI provider did not answer in time", got[0].Error.MustGet())
		assert.Equal(t, "What should I ask?", got[0].Question.MustGet())
	})

	t.Run("the monthly allowance", func(t *testing.T) {
		res := a.ask(token, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var q api.AssistantAnswer
		res.JSON(&q)
		aiAnswers(t, a, aiConn, q.Id, api.AssistantAnsweredJob{
			Answer: nullable.NewNullableWithValue("Summary."), Error: nullable.NewNullNullable[string](),
			Model: nullable.NewNullNullable[string](), InputTokens: 3000, OutputTokens: 600,
		})
		res = a.ask(token, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
		assert.Equal(t, http.StatusTooManyRequests, res.Status)
		assert.Equal(t, api.ErrorCodeASSISTANTLIMITREACHED, res.Error().Error.Code)
	})
}

func TestTheAssistantIsOffUntilAProviderIsSetUp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Ali"})

	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.False(t, status.Enabled)
	res := a.ask(token, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeASSISTANTOFF, res.Error().Error.Code)
}

func TestAssistantQuestionRules(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Ali"})
	res := a.ask(token, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "fit"})
	assert.Equal(t, []string{"kind"}, fieldNames(t, res.Error()))
	res = a.ask(token, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "question"})
	assert.Equal(t, []string{"question"}, fieldNames(t, res.Error()))
	res = a.ask(token, map[string]any{"subjectType": "job", "subjectId": "job_01jbz6w0m0k8q9x2r3s4t5v6w7", "kind": "summary"})
	assert.Equal(t, http.StatusNotFound, res.Status)
}
