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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// aiWritesEmail plays the AI worker for one email: it returns what it was asked.
func aiWritesEmail(t *testing.T, a *testApp, conn *pgx.Conn, draftID string, wrote api.AssistantEmailedJob) api.AssistantEmailJob {
	t.Helper()
	job := claimAIJobWhere(t, conn, "draftId", draftID)
	require.Equal(t, string(api.JobKindAssistantEmail), job.Kind)
	var asked api.AssistantEmailJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	wrote.DraftId = draftID
	answerAIJob(t, a, conn, job, api.JobKindAssistantEmailed, wrote)
	return asked
}

func TestTheAIWritesEmailsWithoutSeeingPay(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance Manager"})
	appl := a.addToJob(token, job.Id, c.Id)

	res := a.request(http.MethodPost, "/v1/assistant/email-drafts", token, map[string]any{
		"mode": "write", "recipient": "candidate", "applicationId": appl.Id,
		"instruction": "Invite ⟦NAME⟧ to interview on Thursday.\nExpected salary: RM 9,999 a month",
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var queued api.AssistantEmailDraft
	res.JSON(&queued)
	assert.Equal(t, api.AssistantEmailStatusQueued, queued.Status)

	asked := aiWritesEmail(t, a, aiConn, queued.Id, api.AssistantEmailedJob{
		Subject: nullable.NewNullableWithValue("Interview on Thursday"), Body: nullable.NewNullableWithValue("Hi ⟦NAME⟧,\n\nCould we meet on Thursday?\n\n⟦ME⟧"),
		Error: nullable.NewNullNullable[string](), Model: nullable.NewNullableWithValue("deepseek-chat"), InputTokens: 400, OutputTokens: 80,
	})
	assert.Equal(t, api.AssistantEmailModeWrite, asked.Mode)
	assert.Contains(t, asked.Instruction.MustGet(), "Invite ⟦NAME⟧ to interview on Thursday.")
	assert.NotContains(t, asked.Instruction.MustGet(), "9,999", "pay never reaches the AI")
	assert.Equal(t, "Finance Manager", asked.JobTitle.MustGet())
	assert.Equal(t, "Maju Bhd", asked.ClientCompany.MustGet())
	assert.NotEmpty(t, asked.Agency)

	var got api.AssistantEmailDraft
	a.request(http.MethodGet, "/v1/assistant/email-drafts/"+queued.Id, token, nil).JSON(&got)
	assert.Equal(t, api.AssistantEmailStatusDone, got.Status)
	assert.Equal(t, "Interview on Thursday", got.Subject.MustGet())
	assert.Equal(t, "Hi ⟦NAME⟧,\n\nCould we meet on Thursday?\n\n⟦ME⟧", got.Body.MustGet())
	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.EqualValues(t, 480, status.UsedTokens, "it counts towards the month's allowance")

	t.Run("only the person it was written for sees it", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/assistant/email-drafts/"+queued.Id, tenant.Users[api.RoleSourcer].Token, nil)
		assert.Equal(t, http.StatusNotFound, res.Status)
	})

	t.Run("each way needs its text", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"write without saying what":   {"mode": "write", "recipient": "client"},
			"improve without a draft":     {"mode": "improve", "recipient": "client", "style": "grammar"},
			"improve without how":         {"mode": "improve", "recipient": "client", "draft": "dear sir"},
			"reply without their email":   {"mode": "reply", "recipient": "client", "instruction": "say yes"},
			"another company's candidate": {"mode": "write", "recipient": "candidate", "instruction": "hi", "applicationId": "appl_00000000000000000000000000"},
		} {
			res := a.request(http.MethodPost, "/v1/assistant/email-drafts", token, body)
			assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, res.Status, "%s: %s", name, res.Body)
		}
	})

	t.Run("a draft is improved, and why it failed is told", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/assistant/email-drafts", token, map[string]any{
			"mode": "improve", "recipient": "client", "style": "formal", "subject": "Shortlist", "draft": "hi ⟦NAME⟧ here r the cvs",
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var q api.AssistantEmailDraft
		res.JSON(&q)
		asked := aiWritesEmail(t, a, aiConn, q.Id, api.AssistantEmailedJob{
			Subject: nullable.NewNullNullable[string](), Body: nullable.NewNullNullable[string](),
			Error: nullable.NewNullableWithValue("the AI provider timed out"), Model: nullable.NewNullNullable[string](),
		})
		assert.Equal(t, api.AssistantEmailStyleFormal, *asked.Style)
		assert.Equal(t, "hi ⟦NAME⟧ here r the cvs", asked.Draft.MustGet())
		var got api.AssistantEmailDraft
		a.request(http.MethodGet, "/v1/assistant/email-drafts/"+q.Id, token, nil).JSON(&got)
		assert.Equal(t, api.AssistantEmailStatusFailed, got.Status)
		assert.Equal(t, "the AI provider timed out", got.Error.MustGet())
	})
}

func TestTheAIEmailHelpIsOffUntilAProviderIsSetUp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	res := a.request(http.MethodPost, "/v1/assistant/email-drafts", tenant.Users[api.RoleRecruiter].Token,
		map[string]any{"mode": "write", "recipient": "candidate", "instruction": "say hello"})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeASSISTANTOFF, res.Error().Error.Code)
	res = a.request(http.MethodPost, "/v1/assistant/email-drafts", tenant.Users[api.RoleViewer].Token,
		map[string]any{"mode": "write", "recipient": "candidate", "instruction": "say hello"})
	assert.Equal(t, http.StatusForbidden, res.Status, "viewers may not use the assistant")
}

// Someone who may email a candidate but not see jobs still gets the AI's
// help: it is just not told the job.
func TestTheAIHelpsWithEmailsWithoutJobsAccess(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, token := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	c := a.createCandidate(owner, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	company := a.createCompany(owner, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(owner, map[string]any{"companyId": company.Id, "title": "Finance Manager"})
	appl := a.addToJob(owner, job.Id, c.Id)
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "candidateScope": "all", "jobCandidates": true,
		"permissions": without(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionPipelineRead, api.PermissionJobsRead)}, http.StatusOK)

	res := a.request(http.MethodPost, "/v1/assistant/email-drafts", token, map[string]any{
		"mode": "write", "recipient": "candidate", "applicationId": appl.Id, "instruction": "Ask ⟦NAME⟧ for her notice period.",
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var queued api.AssistantEmailDraft
	res.JSON(&queued)
	asked := aiWritesEmail(t, a, aiConn, queued.Id, api.AssistantEmailedJob{
		Subject: nullable.NewNullableWithValue("Notice period"), Body: nullable.NewNullableWithValue("Hi ⟦NAME⟧"),
		Error: nullable.NewNullNullable[string](), Model: nullable.NewNullableWithValue("deepseek-chat"),
	})
	assert.False(t, asked.JobTitle.IsSpecified() && !asked.JobTitle.IsNull(), "the job is not told")
	assert.False(t, asked.ClientCompany.IsSpecified() && !asked.ClientCompany.IsNull(), "nor the client")
}
