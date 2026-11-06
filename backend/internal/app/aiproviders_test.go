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

const aiKeysSecret = "test-ai-keys-secret-0123456789-abcdef"

func (a *testApp) aiProviders(token string) api.AiProviders {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/ai-providers", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.AiProviders
	res.JSON(&out)
	return out
}

func (a *testApp) assistantOn(token string) bool {
	a.t.Helper()
	var st api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&st)
	return st.Enabled
}

func entryOf(st api.AiProviders, provider api.AiProviderName) api.AiProviderEntry {
	for _, e := range st.Providers {
		if e.Provider == provider {
			return e
		}
	}
	return api.AiProviderEntry{}
}

// claimCheck claims the AI worker's check of one sealed key.
func claimCheck(t *testing.T, conn *pgx.Conn, sealed string) claimedJob {
	t.Helper()
	claimLock.Lock()
	defer claimLock.Unlock()
	ctx := context.Background()
	_, err := testutil.Current.Admin.Exec(ctx,
		`UPDATE job SET run_after = now() - interval '1 hour' WHERE queue = 'ai' AND payload->'access'->>'sealedKey' = $1`, sealed)
	require.NoError(t, err)
	var j claimedJob
	var reqID *string
	require.NoError(t, conn.QueryRow(ctx, `SELECT id, tenant_id, kind, payload, request_id FROM job_claim('ai', 'python-test', 60)`).
		Scan(&j.ID, &j.TenantID, &j.Kind, &j.Payload, &reqID))
	require.Equal(t, string(api.JobKindAiproviderCheck), j.Kind)
	require.Contains(t, string(j.Payload), sealed)
	if reqID != nil {
		j.RequestID = *reqID
	}
	return j
}

func savedSealed(t *testing.T, tenantID, provider string) string {
	t.Helper()
	var sealed string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT sealed_key FROM ai_provider WHERE tenant_id = $1 AND provider = $2`, tenantID, provider).Scan(&sealed))
	return sealed
}

func TestCompaniesChooseTheirAIProviderWithTheirOwnKeys(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_KEYS_SECRET": aiKeysSecret}) // the server has no AI of its own
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/ai-providers", recruiter, nil).Status, "owners and admins only")
	st := a.aiProviders(owner)
	assert.Equal(t, api.AiProviderChoiceServer, st.Active)
	assert.False(t, st.ServerAvailable)
	assert.True(t, st.KeysSupported)
	require.Len(t, st.Providers, 3)
	assert.Equal(t, []api.AiProviderName{"deepseek", "claude", "openai"}, []api.AiProviderName{st.Providers[0].Provider, st.Providers[1].Provider, st.Providers[2].Provider})
	assert.False(t, a.assistantOn(recruiter))

	res := a.request(http.MethodPut, "/v1/ai-providers/claude", owner, map[string]any{"model": "claude-opus-5-5"})
	assert.Equal(t, http.StatusBadRequest, res.Status, "the first save needs a key: %s", res.Body)

	const key = "sk-ant-api03-secret-value-5678"
	res = a.request(http.MethodPut, "/v1/ai-providers/claude", owner, map[string]any{"model": "claude-opus-5-5", "apiKey": key})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.NotContains(t, string(res.Body), key, "the key is never sent back")
	assert.NotContains(t, string(res.Body), "v1.", "nor sealed")
	res.JSON(&st)
	assert.Equal(t, api.AiProviderChoiceClaude, st.Active, "a company without an AI switches to the first it saves")
	claude := entryOf(st, "claude")
	assert.Equal(t, "…5678", claude.KeyHint.MustGet())
	assert.Equal(t, "claude-opus-5-5", claude.Model.MustGet())
	assert.Equal(t, api.AiProviderCheckStatusQueued, claude.Check.MustGet().Status)
	assert.True(t, a.assistantOn(recruiter))

	sealed := savedSealed(t, tenant.ID, "claude")
	assert.NotContains(t, sealed, key, "stored sealed")
	var audit string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx,
		`SELECT string_agg(changes::text, ' ') FROM audit_log WHERE tenant_id = $1 AND action LIKE 'aiprovider.%'`, tenant.ID).Scan(&audit))
	assert.NotContains(t, audit, key)
	assert.NotContains(t, audit, sealed)
	assert.Contains(t, audit, `"keyChanged": true`)

	t.Run("the AI's jobs carry the company's choice", func(t *testing.T) {
		c := a.createCandidate(recruiter, map[string]any{"firstName": "Ana", "lastName": "Lim", "currentTitle": "Analyst"})
		res := a.ask(recruiter, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var q api.AssistantAnswer
		res.JSON(&q)
		asked := aiAnswers(t, a, aiConn, q.Id, api.AssistantAnsweredJob{
			Answer: nullable.NewNullableWithValue("Summary."), Error: nullable.NewNullNullable[string](), Model: nullable.NewNullNullable[string](),
		})
		require.NotNil(t, asked.Ai)
		assert.Equal(t, api.AiAccess{Provider: "claude", Model: "claude-opus-5-5", SealedKey: sealed}, *asked.Ai)
	})

	t.Run("the check's result is shown; a check of a replaced key is not", func(t *testing.T) {
		job := claimCheck(t, aiConn, sealed)
		var check api.AiProviderCheckJob
		require.NoError(t, json.Unmarshal(job.Payload, &check))
		assert.Equal(t, api.AiAccess{Provider: "claude", Model: "claude-opus-5-5", SealedKey: sealed}, check.Access)

		res := a.request(http.MethodPut, "/v1/ai-providers/claude", owner, map[string]any{"model": "claude-sonnet-5-5", "apiKey": key + "-new"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		answerAIJob(t, a, aiConn, job, api.JobKindAiproviderChecked, api.AiProviderCheckedJob{
			Provider: "claude", SealedKey: sealed, Ok: false, Message: nullable.NewNullableWithValue("the AI provider refused the API key"),
		})
		assert.Equal(t, api.AiProviderCheckStatusQueued, entryOf(a.aiProviders(owner), "claude").Check.MustGet().Status)

		newSealed := savedSealed(t, tenant.ID, "claude")
		answerAIJob(t, a, aiConn, claimCheck(t, aiConn, newSealed), api.JobKindAiproviderChecked, api.AiProviderCheckedJob{
			Provider: "claude", SealedKey: newSealed, Ok: false, Message: nullable.NewNullableWithValue("the AI provider account has no credit left"),
		})
		c := entryOf(a.aiProviders(owner), "claude").Check.MustGet()
		assert.Equal(t, api.AiProviderCheckStatusFailed, c.Status)
		assert.Equal(t, "the AI provider account has no credit left", c.Message.MustGet())

		// Saving the model alone keeps the key.
		res = a.request(http.MethodPut, "/v1/ai-providers/claude", owner, map[string]any{"model": "claude-haiku-5-5"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, newSealed, savedSealed(t, tenant.ID, "claude"))
		answerAIJob(t, a, aiConn, claimCheck(t, aiConn, newSealed), api.JobKindAiproviderChecked, api.AiProviderCheckedJob{
			Provider: "claude", SealedKey: newSealed, Ok: true, Message: nullable.NewNullNullable[string](),
		})
		assert.Equal(t, api.AiProviderCheckStatusOk, entryOf(a.aiProviders(owner), "claude").Check.MustGet().Status)
	})

	t.Run("switching", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/ai-providers", owner, map[string]any{"provider": "openai"})
		require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
		assert.Equal(t, api.ErrorCodeAIPROVIDERNOKEY, res.Error().Error.Code)

		res = a.request(http.MethodPut, "/v1/ai-providers", owner, map[string]any{"provider": "off"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.False(t, a.assistantOn(recruiter))
		c := a.createCandidate(recruiter, map[string]any{"firstName": "Bo", "lastName": "Tan"})
		res = a.ask(recruiter, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
		assert.Equal(t, api.ErrorCodeASSISTANTOFF, res.Error().Error.Code)

		require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/ai-providers", owner, map[string]any{"provider": "claude"}).Status)
		assert.True(t, a.assistantOn(recruiter))
		assert.Equal(t, "claude-haiku-5-5", entryOf(a.aiProviders(owner), "claude").Model.MustGet(), "switching back keeps the model and key")
	})

	t.Run("removing the key in use switches the AI off", func(t *testing.T) {
		res := a.request(http.MethodDelete, "/v1/ai-providers/claude", owner, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var st api.AiProviders
		res.JSON(&st)
		assert.Equal(t, api.AiProviderChoiceOff, st.Active)
		assert.False(t, entryOf(st, "claude").KeyHint.IsSpecified() && !entryOf(st, "claude").KeyHint.IsNull())
		assert.False(t, a.assistantOn(recruiter))
	})

	t.Run("another company sees none of it", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
		st := a.aiProviders(other)
		assert.Equal(t, api.AiProviderChoiceServer, st.Active)
		assert.True(t, entryOf(st, "claude").KeyHint.IsNull())
	})
}

func TestTheServersOwnAIAndKeysThatCannotBeSaved(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn) // the server has its own AI, but no HP_AI_KEYS_SECRET
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token

	st := a.aiProviders(owner)
	assert.True(t, st.ServerAvailable)
	assert.False(t, st.KeysSupported)
	assert.True(t, a.assistantOn(owner), "no choice yet: the server's own AI")

	res := a.request(http.MethodPut, "/v1/ai-providers/deepseek", owner, map[string]any{"model": "deepseek-v4-flash", "apiKey": "sk-0123456789abcdef"})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeAIKEYSNOTSETUP, res.Error().Error.Code)

	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/ai-providers", owner, map[string]any{"provider": "off"}).Status)
	assert.False(t, a.assistantOn(owner), "a company can switch the server's AI off for itself")
}
