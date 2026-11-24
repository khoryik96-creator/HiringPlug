package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// remainingAI is what aiusage.Remaining says for the company, as a module
// checking the allowance would ask it.
func (a *testApp) remainingAI(tenantID string) int64 {
	a.t.Helper()
	var left int64
	require.NoError(a.t, database.InTx(context.Background(), a.deps.Pool, authz.TenantOnly(tenantID), func(tx pgx.Tx) error {
		var err error
		left, err = a.modules.AIUsage.Remaining(context.Background(), tx, tenantID)
		return err
	}))
	return left
}

// pendingAIJob leaves an AI job waiting for the company (a 1,000-token hold).
// It is due tomorrow, so no other test's AI worker claims it.
func pendingAIJob(t *testing.T, tenantID string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(), `INSERT INTO job (tenant_id, queue, kind, payload, run_after, priority)
		VALUES ($1, 'ai', 'assistant.ask', '{}', now() + interval '1 day', 9)`, tenantID)
	require.NoError(t, err)
}

func TestTheAIAllowanceInTokensDefaultsToTheServers(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_MONTHLY_TOKENS": "5000"})
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	other := testutil.Current.NewTenant(t, api.RoleOwner)

	assert.Equal(t, int64(5000), a.remainingAI(tenant.ID), "nothing used: the server's whole default")
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureAssistant, SourceID: "ask_a", Input: 600, Output: 400})
	assert.Equal(t, int64(4000), a.remainingAI(tenant.ID))
	pendingAIJob(t, tenant.ID)
	assert.Equal(t, int64(3000), a.remainingAI(tenant.ID), "work in progress holds 1,000 tokens")
	// Last month's use and another company's use never count.
	usedEarlier(t, tenant.ID, aiusage.FeatureFitScore, "jm_old", 50_000, 0, agencyMonth(t).Add(-1))
	a.recordAIUse(other.ID, aiusage.Use{Feature: aiusage.FeatureAssistant, SourceID: "ask_b", Input: 4000})
	assert.Equal(t, int64(3000), a.remainingAI(tenant.ID))
	assert.Equal(t, int64(1000), a.remainingAI(other.ID))

	// The company's own limit replaces the server's.
	res := a.request(http.MethodPut, "/v1/ai-usage/settings", tenant.Users[api.RoleOwner].Token, tokenLimit(2500, 0))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, int64(500), a.remainingAI(tenant.ID))
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", tenant.Users[api.RoleOwner].Token, tokenLimit(1500, 1))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, int64(-500), a.remainingAI(tenant.ID), "over the limit: nothing left")
	assert.Equal(t, int64(1000), a.remainingAI(other.ID), "the other company keeps the server's default")
}

func TestTheAIAllowanceInMoney(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_MONTHLY_TOKENS": "5000"})
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	// 10.00 a month at 2.00 per million input tokens and 8.00 per million output.
	res := a.request(http.MethodPut, "/v1/ai-usage/settings", tenant.Users[api.RoleOwner].Token, map[string]any{"limitKind": "money",
		"limitTokens": nil, "limitMoney": "10.00", "currency": "USD", "priceInputPerMillion": "2", "priceOutputPerMillion": "8", "warnPercent": 80, "version": 0})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, int64(1_250_000), a.remainingAI(tenant.ID), "10.00 at the higher price")

	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureFitScore, SourceID: "jm_a", RunKey: "1", Input: 1_000_000, Output: 250_000})
	assert.Equal(t, int64(750_000), a.remainingAI(tenant.ID), "4.00 spent, 6.00 left")
	pendingAIJob(t, tenant.ID)
	assert.Equal(t, int64(749_000), a.remainingAI(tenant.ID), "the hold is reserved at the higher price")
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureFitScore, SourceID: "jm_a", RunKey: "2", Input: 2_996_000})
	assert.Equal(t, int64(0), a.remainingAI(tenant.ID), "exactly used up, the hold included")
}

func TestRecordingAIUse(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].ID
	system := authz.SystemUserID
	ledger := func() (rows int, input, output int64) {
		t.Helper()
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*), coalesce(sum(input_tokens), 0), coalesce(sum(output_tokens), 0)
			FROM ai_use WHERE tenant_id = $1`, tenant.ID).Scan(&rows, &input, &output))
		return rows, input, output
	}

	use := aiusage.Use{Feature: aiusage.FeatureCVReading, UserID: &recruiter, SourceID: "doc_a", RunKey: "job_1", Input: 700, Output: 50}
	a.recordAIUse(tenant.ID, use)
	a.recordAIUse(tenant.ID, use) // the same result delivered again
	rows, in, out := ledger()
	assert.Equal(t, 1, rows, "a repeated result counts once")
	assert.Equal(t, int64(700), in)
	assert.Equal(t, int64(50), out)

	use.RunKey = "job_2" // the same CV read again
	a.recordAIUse(tenant.ID, use)
	rows, _, _ = ledger()
	assert.Equal(t, 2, rows, "a new run of the same record counts")

	// Counts are kept in range; nothing used adds nothing; the system's own
	// work has no user.
	a.recordAIUse(tenant.ID,
		aiusage.Use{Feature: aiusage.FeatureInterviewPrep, UserID: &system, SourceID: "ivp_a", Input: -40, Output: 1 << 40},
		aiusage.Use{Feature: aiusage.FeatureAssistant, SourceID: "ask_empty"},
	)
	var user *string
	var input, output int64
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT user_id, input_tokens, output_tokens FROM ai_use WHERE tenant_id = $1 AND source_id = 'ivp_a'`,
		tenant.ID).Scan(&user, &input, &output))
	assert.Nil(t, user)
	assert.Equal(t, int64(0), input)
	assert.Equal(t, int64(aiusage.MaxTokens), output)
	rows, _, _ = ledger()
	assert.Equal(t, 3, rows)

	// A user's own request transaction can record too (as the system, then
	// back to the user), and an unknown feature is refused.
	var p authz.Principal
	require.NoError(t, database.InTx(ctx, a.deps.Pool, authz.TenantOnly(tenant.ID), func(tx pgx.Tx) error {
		var err error
		p, _, err = a.modules.Identity.PrincipalFor(ctx, tx, recruiter)
		return err
	}))
	require.False(t, p.Can(api.PermissionAiusageRead), "a recruiter cannot read the ledger")
	require.NoError(t, database.InTx(ctx, a.deps.Pool, p, func(tx pgx.Tx) error {
		if err := a.modules.AIUsage.Record(ctx, tx, tenant.ID, aiusage.Use{Feature: aiusage.FeatureProviderCheck, UserID: &recruiter, SourceID: "chk_a", Input: 5}); err != nil {
			return err
		}
		var me string
		if err := tx.QueryRow(ctx, `SELECT app_current_user()`).Scan(&me); err != nil {
			return err
		}
		assert.Equal(t, recruiter, me, "the caller's scope is back")
		return nil
	}))
	require.Error(t, database.InTx(ctx, a.deps.Pool, authz.TenantOnly(tenant.ID), func(tx pgx.Tx) error {
		return a.modules.AIUsage.Record(ctx, tx, tenant.ID, aiusage.Use{Feature: "chat", SourceID: "x", Input: 1})
	}))
	rows, _, _ = ledger()
	assert.Equal(t, 4, rows)

	// Nobody can change or delete a use, even in their own company.
	require.NoError(t, database.InTx(ctx, a.deps.Pool, authz.System(tenant.ID), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE ai_use SET input_tokens = 0`)
		assert.Equal(t, int64(0), tag.RowsAffected())
		if err != nil {
			return err
		}
		tag, err = tx.Exec(ctx, `DELETE FROM ai_use`)
		assert.Equal(t, int64(0), tag.RowsAffected())
		return err
	}))
	rows, _, _ = ledger()
	assert.Equal(t, 4, rows)
}
