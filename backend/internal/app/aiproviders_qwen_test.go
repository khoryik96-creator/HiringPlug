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

// Qwen is a fourth provider, like the others: its own key and model, checked
// by the AI worker, and chosen for the company's AI.
func TestCompaniesCanUseQwenWithTheirOwnKey(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_KEYS_SECRET": aiKeysSecret})
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	assert.Equal(t, api.AiProviderName("qwen"), a.aiProviders(owner).Providers[3].Provider, "listed last")
	const key = "sk-qwen-0123456789abcdef"
	res := a.request(http.MethodPut, "/v1/ai-providers/qwen", owner, map[string]any{"model": "qwen-plus", "apiKey": key})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.NotContains(t, string(res.Body), key)
	var st api.AiProviders
	res.JSON(&st)
	assert.Equal(t, api.AiProviderChoiceQwen, st.Active, "a company without an AI switches to the first it saves")
	assert.Equal(t, "…cdef", entryOf(st, "qwen").KeyHint.MustGet())
	assert.True(t, a.assistantOn(recruiter))

	sealed := savedSealed(t, tenant.ID, "qwen")
	job := claimCheck(t, aiConn, sealed)
	var check api.AiProviderCheckJob
	require.NoError(t, json.Unmarshal(job.Payload, &check))
	assert.Equal(t, api.AiProviderName("qwen"), check.Access.Provider, "the worker is told to call Qwen")
	assert.Equal(t, "qwen-plus", check.Access.Model)
	answerAIJob(t, a, aiConn, job, api.JobKindAiproviderChecked, api.AiProviderCheckedJob{
		Provider: "qwen", SealedKey: sealed, Ok: true, Message: nullable.NewNullNullable[string](),
	})
	assert.Equal(t, api.AiProviderCheckStatusOk, entryOf(a.aiProviders(owner), "qwen").Check.MustGet().Status)

	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/ai-providers", owner, map[string]any{"provider": "off"}).Status)
	res = a.request(http.MethodPut, "/v1/ai-providers", owner, map[string]any{"provider": "qwen"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, api.AiProviderChoiceQwen, a.aiProviders(owner).Active)
}
