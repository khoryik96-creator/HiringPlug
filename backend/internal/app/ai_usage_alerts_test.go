package app_test

// When this month's AI use reaches the company's warning level, and then
// its whole limit, the people who may see the company's AI use get one bell
// item per level (only the limit's once it is reached), in the morning email
// too; each level is audited once.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// aiLimitItems is the user's AI limit notifications.
func (a *testApp) aiLimitItems(token string) []api.Notification {
	a.t.Helper()
	var out []api.Notification
	for _, it := range a.notifications(token).Items {
		if it.Kind == api.NotificationKindAiLimit {
			out = append(out, it)
		}
	}
	return out
}

// limitAudits is the alert levels audited for the company: action -> changes.
func limitAudits(t *testing.T, tenantID string) map[string][]map[string]any {
	t.Helper()
	rows, err := testutil.Current.Admin.Query(context.Background(), `SELECT action, changes FROM audit_log
		WHERE tenant_id = $1 AND action IN ('ai_usage.warning_reached', 'ai_usage.limit_reached') ORDER BY created_at`, tenantID)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string][]map[string]any{}
	for rows.Next() {
		var action string
		var raw []byte
		require.NoError(t, rows.Scan(&action, &raw))
		var changes map[string]any
		require.NoError(t, json.Unmarshal(raw, &changes))
		out[action] = append(out[action], changes)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestTheAILimitWarnsWhoMaySeeTheCompanysAIUse(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	month := agencyMonth(t)
	monthKey, until := month.Format("2006-01"), month.AddDate(0, 1, 0).Format("2 Jan")

	// 10,000 tokens a month, warned at 80%.
	res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner.Token, tokenLimit(10_000, 0))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureFitScore, SourceID: "jm_a", Input: 7000})
	assert.Empty(t, a.aiLimitItems(owner.Token), "70%: nothing yet")

	// 80%: the warning, once.
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureFitScore, SourceID: "jm_b", Input: 1000})
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureFitScore, SourceID: "jm_c", Input: 500})
	items := a.aiLimitItems(owner.Token)
	require.Len(t, items, 1)
	assert.Equal(t, "ai_limit:"+monthKey+":80", items[0].Id)
	assert.Equal(t, "AI use reached 80% of this month's limit", items[0].Title)
	assert.Equal(t, "#/settings/ai-usage", items[0].Href)
	assert.True(t, items[0].New)
	assert.Len(t, a.aiLimitItems(admin), 1, "admins may see the company's AI use too")
	assert.Empty(t, a.aiLimitItems(recruiter), "a recruiter may not")
	audits := limitAudits(t, tenant.ID)
	assert.Equal(t, []map[string]any{{"percent": float64(80), "month": monthKey}}, audits["ai_usage.warning_reached"], "the percent and month only")
	assert.Empty(t, audits["ai_usage.limit_reached"])

	// The owner looks; then an assistant answer uses the rest.
	seen := a.notifications(owner.Token)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/notifications/seen", owner.Token, map[string]any{"upTo": seen.AsOf}).Status)
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Nadia", "lastName": "Rahim"})
	res = a.ask(recruiter, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
	require.Equal(t, http.StatusAccepted, res.Status, "1,500 tokens were left: %s", res.Body)
	var queued api.AssistantAnswer
	res.JSON(&queued)
	answer := api.AssistantAnsweredJob{Answer: nullable.NewNullableWithValue("Six years in finance."), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullNullable[string](), InputTokens: 1200, OutputTokens: 300}
	aiAnswers(t, a, aiConn, queued.Id, answer)

	items = a.aiLimitItems(owner.Token)
	require.Len(t, items, 1, "once the limit is reached only that shows")
	assert.Equal(t, "ai_limit:"+monthKey+":100", items[0].Id)
	assert.Equal(t, "AI limit reached: AI features are paused until "+until, items[0].Title)
	assert.True(t, items[0].New, "reached after the owner looked")
	assert.Empty(t, a.aiLimitItems(recruiter))
	var status api.AiUsageStatus
	res = a.request(http.MethodGet, "/v1/ai-usage/status", recruiter, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&status)
	assert.True(t, status.Reached, "everyone can see the limit is reached (for the banner)")

	// More use this month adds no notice and no audit entry.
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureProviderCheck, SourceID: "chk_a", Input: 40})
	assert.Len(t, a.aiLimitItems(owner.Token), 1)
	audits = limitAudits(t, tenant.ID)
	assert.Len(t, audits["ai_usage.warning_reached"], 1)
	assert.Equal(t, []map[string]any{{"percent": float64(100), "month": monthKey}}, audits["ai_usage.limit_reached"])

	t.Run("muted", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/notifications/settings", admin, map[string]any{"morningEmail": false, "mutedKinds": []string{"ai_limit"}})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Empty(t, a.aiLimitItems(admin), "turned off")
		assert.Len(t, a.aiLimitItems(owner.Token), 1, "only for who turned it off")
	})

	t.Run("in the morning email", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/notifications/settings", owner.Token, map[string]any{"morningEmail": true, "mutedKinds": []string{}})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		digestDueNow(t, a, tenant.ID)
		msgs := mailServer.To("owner." + tenant.ID + "@test.local")
		require.Len(t, msgs, 1)
		assert.Contains(t, msgs[0].Data, "1 AI limit notice")
		assert.Contains(t, msgs[0].Data, "AI limit reached: AI features are paused until "+until)
		assert.Contains(t, msgs[0].Data, "/#/settings/ai-usage")
	})

	t.Run("another company sees none", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleOwner)
		assert.Empty(t, a.aiLimitItems(other.Users[api.RoleOwner].Token))
	})
}

// Two results landing together that cross the limit together: the second
// to finish sees the first's use, so the limit is noted.
func TestTwoAIResultsCrossingTheLimitTogetherAreNoted(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance) // 1,000,000 tokens a month
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureCVStudio, SourceID: "scv_a", Input: 960_000})

	record := func(source string) error {
		return database.InTx(context.Background(), a.deps.Pool, authz.TenantOnly(tenant.ID), func(tx pgx.Tx) error {
			return a.modules.AIUsage.Record(context.Background(), tx, tenant.ID, aiusage.Use{Feature: aiusage.FeatureCVStudio, SourceID: source, Input: 30_000})
		})
	}
	// The first result is recorded but not yet committed when the second lands.
	recorded, release, firstDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		firstDone <- database.InTx(context.Background(), a.deps.Pool, authz.TenantOnly(tenant.ID), func(tx pgx.Tx) error {
			err := a.modules.AIUsage.Record(context.Background(), tx, tenant.ID, aiusage.Use{Feature: aiusage.FeatureCVStudio, SourceID: "scv_b", Input: 30_000})
			close(recorded)
			<-release
			return err
		})
	}()
	<-recorded
	secondDone := make(chan error, 1)
	go func() { secondDone <- record("scv_c") }()
	select {
	case err := <-secondDone: // it did not wait for the first
		close(release)
		require.NoError(t, err)
		require.NoError(t, <-firstDone)
	case <-time.After(300 * time.Millisecond): // it waits for the first to end
		close(release)
		require.NoError(t, <-firstDone)
		require.NoError(t, <-secondDone)
	}

	assert.Equal(t, 1, countRows(t, `SELECT count(*) FROM ai_usage_alert WHERE tenant_id = $1 AND level = 100`, tenant.ID),
		"1,020,000 of 1,000,000 used: the limit is reached")
	assert.Len(t, limitAudits(t, tenant.ID)["ai_usage.limit_reached"], 1)
}

// The notices follow the limit: raised past this month's use, the "limit
// reached" notice goes, and reaching the new limit is announced again;
// lowered below it, the limit is announced at once (no AI work can start
// to announce it later).
func TestTheAILimitNoticesFollowTheCompanysLimit(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	monthKey := agencyMonth(t).Format("2006-01")
	save := func(tokens int64) {
		t.Helper()
		version := a.aiUsageSettings(owner).Version
		res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner, tokenLimit(tokens, version))
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}

	// Lowered below this month's use: announced when saved.
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureAssistant, SourceID: "ask_a", Input: 6000})
	assert.Empty(t, a.aiLimitItems(owner), "6,000 of 1,000,000")
	save(5000)
	items := a.aiLimitItems(owner)
	require.Len(t, items, 1)
	assert.Equal(t, "ai_limit:"+monthKey+":100", items[0].Id)
	assert.Len(t, limitAudits(t, tenant.ID)["ai_usage.limit_reached"], 1)

	// Raised: AI works again, and the notice goes.
	save(50_000)
	assert.Empty(t, a.aiLimitItems(owner), "6,000 of 50,000: neither the limit nor the warning")
	seen := a.notifications(owner)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/notifications/seen", owner, map[string]any{"upTo": seen.AsOf}).Status)

	// The new limit reached: announced again, as new.
	a.recordAIUse(tenant.ID, aiusage.Use{Feature: aiusage.FeatureAssistant, SourceID: "ask_b", Input: 44_000})
	items = a.aiLimitItems(owner)
	require.Len(t, items, 1)
	assert.Equal(t, "ai_limit:"+monthKey+":100", items[0].Id)
	assert.True(t, items[0].New, "reached after the owner looked")
	assert.Len(t, limitAudits(t, tenant.ID)["ai_usage.limit_reached"], 2, "each time the limit is reached")
}
