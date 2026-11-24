package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// recordAIUse records uses as a result job would: in a transaction of the
// company with no permissions (the system's work, or any user's).
func (a *testApp) recordAIUse(tenantID string, uses ...aiusage.Use) {
	a.t.Helper()
	require.NoError(a.t, database.InTx(context.Background(), a.deps.Pool, authz.TenantOnly(tenantID), func(tx pgx.Tx) error {
		for _, u := range uses {
			if err := a.modules.AIUsage.Record(context.Background(), tx, tenantID, u); err != nil {
				return err
			}
		}
		return nil
	}))
}

// usedEarlier adds a use at a past time, as one recorded then would be.
func usedEarlier(t *testing.T, tenantID, feature, sourceID string, input, output int64, at time.Time) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(), `INSERT INTO ai_use (tenant_id, feature, source_id, input_tokens, output_tokens, used_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, tenantID, feature, sourceID, input, output, at)
	require.NoError(t, err)
}

func (a *testApp) aiUsage(token, query string) api.AiUsage {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/ai-usage"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var u api.AiUsage
	res.JSON(&u)
	return u
}

// agencyMonth is this month's start in the agency's time zone (the test default).
func agencyMonth(t *testing.T) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	return aiusage.MonthStart(time.Now(), loc)
}

func TestAIUsageReport(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_MONTHLY_TOKENS": "10000000"})
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	month := agencyMonth(t)

	a.recordAIUse(tenant.ID,
		aiusage.Use{Feature: aiusage.FeatureAssistant, UserID: &recruiter.ID, SourceID: "ask_one", Input: 1_000_000, Output: 250_000},
		aiusage.Use{Feature: aiusage.FeatureAssistant, UserID: &recruiter.ID, SourceID: "ask_two", Input: 500_000},
		aiusage.Use{Feature: aiusage.FeatureFitScore, SourceID: "jm_auto", RunKey: "run-1", Input: 2_000_000, Output: 500_000},
		aiusage.Use{Feature: aiusage.FeatureJDReading, UserID: &owner.ID, SourceID: "jdf_one", Input: 100_000},
	)
	usedEarlier(t, tenant.ID, aiusage.FeatureCVReading, "doc_old", 300_000, 0, month.AddDate(0, -2, 3))
	// Another company's use is not in this report.
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a.recordAIUse(other.ID, aiusage.Use{Feature: aiusage.FeatureAssistant, SourceID: "ask_other", Input: 9_000_000})

	u := a.aiUsage(owner.Token, "?months=3")
	assert.Equal(t, aiusage.MonthKey(month, month.Location()), u.Month)
	assert.Equal(t, int64(4), u.Uses)
	assert.Equal(t, int64(3_600_000), u.InputTokens)
	assert.Equal(t, int64(750_000), u.OutputTokens)
	assert.False(t, u.PricesSet)
	assert.True(t, u.Cost.IsNull(), "no prices: no cost")

	// By feature, most tokens first.
	require.Len(t, u.ByFeature, 3)
	assert.Equal(t, api.AiFeatureFitScore, u.ByFeature[0].Feature)
	assert.Equal(t, api.AiUsageFeature{Feature: api.AiFeatureAssistant, Uses: 2, InputTokens: 1_500_000, OutputTokens: 250_000, Cost: u.ByFeature[1].Cost}, u.ByFeature[1])
	assert.True(t, u.ByFeature[1].Cost.IsNull())
	assert.Equal(t, api.AiFeatureJdReading, u.ByFeature[2].Feature)

	// By person: the system's own work is Automatic.
	require.Len(t, u.ByPerson, 3)
	assert.True(t, u.ByPerson[0].UserId.IsNull())
	assert.Equal(t, "Automatic", u.ByPerson[0].Name)
	assert.Equal(t, int64(2_500_000), u.ByPerson[0].Tokens)
	assert.Equal(t, recruiter.ID, u.ByPerson[1].UserId.MustGet())
	assert.NotEmpty(t, u.ByPerson[1].Name)
	assert.NotEqual(t, "Automatic", u.ByPerson[1].Name)
	assert.Equal(t, int64(2), u.ByPerson[1].Uses)
	assert.Equal(t, int64(1_750_000), u.ByPerson[1].Tokens)
	assert.Equal(t, owner.ID, u.ByPerson[2].UserId.MustGet())

	// Months: newest first, an empty month listed.
	require.Len(t, u.Months, 3)
	assert.Equal(t, u.Month, u.Months[0].Month)
	assert.Equal(t, int64(4), u.Months[0].Uses)
	assert.Equal(t, u.ByFeature, u.Months[0].ByFeature)
	assert.Equal(t, aiusage.MonthKey(month.AddDate(0, -1, 0), month.Location()), u.Months[1].Month)
	assert.Equal(t, int64(0), u.Months[1].Uses)
	assert.Empty(t, u.Months[1].ByFeature)
	assert.Equal(t, aiusage.MonthKey(month.AddDate(0, -2, 0), month.Location()), u.Months[2].Month)
	assert.Equal(t, int64(300_000), u.Months[2].InputTokens)
	assert.Equal(t, api.AiFeatureCvReading, u.Months[2].ByFeature[0].Feature)
	assert.Len(t, a.aiUsage(owner.Token, "").Months, 12, "12 months unless asked")
	assert.Len(t, a.aiUsage(owner.Token, "?months=1").Months, 1)
	for _, bad := range []string{"?months=0", "?months=25"} {
		res := a.request(http.MethodGet, "/v1/ai-usage"+bad, owner.Token, nil)
		assert.Equal(t, http.StatusBadRequest, res.Status, bad)
	}

	// The limit: the server's default in tokens.
	assert.Equal(t, api.AiUsageLimitKindTokens, u.Limit.Kind)
	assert.Equal(t, int64(10_000_000), u.Limit.Tokens.MustGet())
	assert.True(t, u.Limit.Money.IsNull())
	assert.Equal(t, 43, u.Limit.UsedPercent) // 4,350,000 of 10,000,000
	assert.False(t, u.Limit.Reached)
	assert.Equal(t, 80, u.Limit.WarnPercent)
	assert.True(t, month.AddDate(0, 1, 0).Equal(u.Limit.ResetsAt))

	// With prices (2.00 / 8.00 per million), each row's cost at them.
	res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner.Token, map[string]any{"limitKind": "money", "limitTokens": nil,
		"limitMoney": "100", "currency": "MYR", "priceInputPerMillion": "2", "priceOutputPerMillion": "8", "warnPercent": 80, "version": 0})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	u = a.aiUsage(owner.Token, "?months=3")
	assert.True(t, u.PricesSet)
	assert.Equal(t, "13.2000", u.Cost.MustGet()) // 3.6M in at 2.00 + 0.75M out at 8.00
	assert.Equal(t, "8.0000", u.ByFeature[0].Cost.MustGet())
	assert.Equal(t, "5.0000", u.ByFeature[1].Cost.MustGet())
	assert.Equal(t, "0.2000", u.ByFeature[2].Cost.MustGet())
	assert.Equal(t, "8.0000", u.ByPerson[0].Cost.MustGet())
	assert.Equal(t, "5.0000", u.ByPerson[1].Cost.MustGet())
	assert.Equal(t, "0.0000", u.Months[1].Cost.MustGet())
	assert.Equal(t, "0.6000", u.Months[2].Cost.MustGet())
	assert.Equal(t, api.AiUsageLimitKindMoney, u.Limit.Kind)
	assert.True(t, u.Limit.Tokens.IsNull())
	assert.Equal(t, "100.00", u.Limit.Money.MustGet())
	assert.Equal(t, "MYR", u.Limit.Currency)
	assert.Equal(t, 13, u.Limit.UsedPercent)
}

func TestAIUsageStatusForEveryone(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_MONTHLY_TOKENS": "1000"})
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	month := agencyMonth(t)
	status := func(token string) map[string]any {
		t.Helper()
		res := a.request(http.MethodGet, "/v1/ai-usage/status", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		return res.Map()
	}
	resets := month.AddDate(0, 1, 0).UTC().Format(time.RFC3339)
	parse := func(s any) string {
		at, err := time.Parse(time.RFC3339Nano, s.(string))
		require.NoError(t, err)
		return at.UTC().Format(time.RFC3339)
	}

	s := status(recruiter)
	assert.Len(t, s, 4, "no token counts or money: %v", s)
	assert.InDelta(t, 0, s["usedPercent"], 0)
	assert.Equal(t, false, s["warn"])
	assert.Equal(t, false, s["reached"])
	assert.Equal(t, resets, parse(s["resetsAt"]))

	// Last month's use does not count; 80% of the server's 1,000 warns.
	usedEarlier(t, tenant.ID, aiusage.FeatureAssistant, "ask_last_month", 5000, 0, month.Add(-time.Minute))
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureAssistant, SourceID: "ask_a", Input: 600, Output: 200})
	s = status(tenant.Users[api.RoleViewer].Token)
	assert.InDelta(t, 80, s["usedPercent"], 0)
	assert.Equal(t, true, s["warn"])
	assert.Equal(t, false, s["reached"])

	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureFitScore, SourceID: "jm_a", Input: 250})
	s = status(recruiter)
	assert.InDelta(t, 105, s["usedPercent"], 0)
	assert.Equal(t, false, s["warn"], "reached, so no longer just a warning")
	assert.Equal(t, true, s["reached"])

	// Another company is untouched.
	other := testutil.Current.NewTenant(t, api.RoleRecruiter)
	assert.Equal(t, false, status(other.Users[api.RoleRecruiter].Token)["reached"])

	// Signed-out callers get nothing.
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/ai-usage/status", "", nil).Status)
}
