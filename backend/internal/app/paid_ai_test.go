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

// IMP-001: one switch pauses every paid AI call on the server, a company's
// own key included ("ai: off, company_keys: on" kept spending before).
// Only the agency that runs the server may turn it; CVs are still read, by
// the rules. Not parallel: the switch is the whole server's (Go runs this
// before any parallel test, which may use the AI).
func TestPausingPaidAIStopsCompanyKeysToo(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	a := newTestApp(t, map[string]string{"HP_AI_KEYS_SECRET": aiKeysSecret, "HP_BACKUP_TENANT": tenant.ID})
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `UPDATE ai_paid_control SET paused = false, reason = NULL`)
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	})
	res := a.request(http.MethodPut, "/v1/ai-providers/claude", owner, map[string]any{"model": "claude-opus-5-5", "apiKey": "sk-ant-api03-synthetic-0000"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/ai-providers", owner, map[string]any{"provider": "claude"}).Status)
	require.True(t, a.assistantOn(recruiter), "the company's own key is in use")

	other := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/settings/system/paid-ai", other, map[string]any{"paused": true}).Status,
		"another agency on the server cannot stop everyone's AI")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/settings/system/paid-ai", recruiter, map[string]any{"paused": true}).Status)
	var st api.SystemStatus
	res = a.request(http.MethodPut, "/v1/settings/system/paid-ai", owner, map[string]any{"paused": true, "reason": "Spend check"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	assert.Equal(t, api.PaidAiSwitch{Paused: true, Reason: st.PaidAi.Reason, ChangedAt: st.PaidAi.ChangedAt, CanChange: true}, st.PaidAi)
	assert.Equal(t, "Spend check", st.PaidAi.Reason.MustGet())

	assert.False(t, a.assistantOn(recruiter), "no AI through the company's key while paused")
	var status api.AiStatus
	a.request(http.MethodGet, "/v1/ai-status", recruiter, nil).JSON(&status)
	assert.True(t, status.PaidPaused, "the notice says so")
	res = a.request(http.MethodPost, "/v1/ai-providers/claude/check", owner, nil)
	assert.Equal(t, http.StatusConflict, res.Status)
	assert.Contains(t, string(res.Body), "AI_PAID_PAUSED")

	// A CV is still read: by the rules alone.
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Siti"})
	res = a.upload("/v1/candidates/"+c.Id+"/documents", recruiter, "original", "cv.pdf", pdfWith("paid-ai"))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, parsedCV))
	parse := claimAIJobFor(t, conn, doc.Id)
	var payload api.DocumentParseCvJob
	require.NoError(t, json.Unmarshal(parse.Payload, &payload))
	assert.False(t, *payload.UseAi)
	assert.Nil(t, payload.Ai, "no key goes to the worker")

	res = a.request(http.MethodPut, "/v1/settings/system/paid-ai", owner, map[string]any{"paused": false})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.True(t, a.assistantOn(recruiter), "back on")
	var actions []string
	rows, err := testutil.Current.Admin.Query(ctx, `SELECT action FROM audit_log WHERE tenant_id = $1 AND action LIKE 'settings.paid_ai%' ORDER BY id`, tenant.ID)
	require.NoError(t, err)
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		actions = append(actions, s)
	}
	assert.Equal(t, []string{"settings.paid_ai_paused", "settings.paid_ai_resumed"}, actions)
}

// HP_AI_PAID=off pauses paid AI whatever the switch says.
func TestHPAIPaidOffPausesPaidAI(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_PROVIDER": "openai_compatible", "HP_AI_PAID": "off"})
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	assert.False(t, a.assistantOn(owner), "the server's own provider is paused too")
	var st api.SystemStatus
	a.request(http.MethodGet, "/v1/settings/system", owner, nil).JSON(&st)
	assert.True(t, st.PaidAi.Paused)
	assert.True(t, st.PaidAi.PausedByServer)
	on := newTestApp(t, map[string]string{"HP_AI_PROVIDER": "openai_compatible"})
	assert.True(t, on.assistantOn(owner), "without it, the server's provider answers")
}
