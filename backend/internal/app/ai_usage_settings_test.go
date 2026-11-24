package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) aiUsageSettings(token string) api.AiUsageSettings {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/ai-usage/settings", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var s api.AiUsageSettings
	res.JSON(&s)
	return s
}

// tokenLimit is a settings body for a limit in tokens (nil: the server's).
func tokenLimit(tokens any, version int) map[string]any {
	return map[string]any{"limitKind": "tokens", "limitTokens": tokens, "limitMoney": nil, "currency": "USD",
		"priceInputPerMillion": nil, "priceOutputPerMillion": nil, "warnPercent": 80, "version": version}
}

func TestAIUsageSettings(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_AI_MONTHLY_TOKENS": "5000"})
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token

	// Before anything is saved: the server's default in tokens, no prices.
	s := a.aiUsageSettings(owner)
	assert.Equal(t, api.AiUsageLimitKindTokens, s.LimitKind)
	assert.True(t, s.LimitTokens.IsNull())
	assert.Equal(t, int64(5000), s.ServerDefaultTokens)
	assert.True(t, s.LimitMoney.IsNull())
	assert.True(t, s.PriceInputPerMillion.IsNull())
	assert.Equal(t, "USD", s.Currency)
	assert.Equal(t, 80, s.WarnPercent)
	assert.Equal(t, 0, s.Version)

	// Only owners and admins see or change them.
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/ai-usage/settings", recruiter, nil).Status)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/ai-usage/settings", recruiter, tokenLimit(1000, 0)).Status)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/ai-usage", recruiter, nil).Status)

	// A limit in money needs the amount and both prices, one above 0.
	res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner, map[string]any{"limitKind": "money", "limitTokens": nil, "limitMoney": nil,
		"currency": "MYR", "priceInputPerMillion": nil, "priceOutputPerMillion": nil, "warnPercent": 80, "version": 0})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.ElementsMatch(t, []string{"limitMoney", "priceInputPerMillion", "priceOutputPerMillion"}, fieldNames(t, res.Error()))
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", owner, map[string]any{"limitKind": "money", "limitTokens": nil, "limitMoney": "50",
		"currency": "MYR", "priceInputPerMillion": "0", "priceOutputPerMillion": "0.00", "warnPercent": 80, "version": 0})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"priceOutputPerMillion"}, fieldNames(t, res.Error()))
	// Out-of-range values are refused by the contract.
	for _, body := range []map[string]any{
		{"warnPercent": 100}, {"warnPercent": 0}, {"currency": "usd"}, {"limitTokens": -1}, {"limitMoney": "1.234"}, {"priceInputPerMillion": "0.12345"},
	} {
		bad := tokenLimit(nil, 0)
		for k, v := range body {
			bad[k] = v
		}
		res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner, bad)
		require.Equal(t, http.StatusBadRequest, res.Status, "%v: %s", body, res.Body)
		assert.Equal(t, api.ErrorCodeVALIDATIONFAILED, res.Error().Error.Code)
	}
	assert.Equal(t, 0, a.aiUsageSettings(owner).Version, "nothing was saved")

	// The first save.
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", owner, map[string]any{"limitKind": "money", "limitTokens": 3000, "limitMoney": "50",
		"currency": "MYR", "priceInputPerMillion": "0.27", "priceOutputPerMillion": "1.1", "warnPercent": 90, "version": 0})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&s)
	assert.Equal(t, api.AiUsageLimitKindMoney, s.LimitKind)
	assert.Equal(t, int64(3000), s.LimitTokens.MustGet())
	assert.Equal(t, "50.00", s.LimitMoney.MustGet())
	assert.Equal(t, "MYR", s.Currency)
	assert.Equal(t, "0.2700", s.PriceInputPerMillion.MustGet())
	assert.Equal(t, "1.1000", s.PriceOutputPerMillion.MustGet())
	assert.Equal(t, 90, s.WarnPercent)
	assert.Equal(t, 1, s.Version)
	assert.False(t, s.UpdatedAt.IsNull())
	assert.Equal(t, s, a.aiUsageSettings(admin))

	// Someone who loaded the settings before that save cannot overwrite it.
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", admin, tokenLimit(nil, 0))
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeCONFLICTSTALEVERSION, res.Error().Error.Code)
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", admin, tokenLimit(nil, 5))
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)

	// An admin changes it back to the server's default in tokens.
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", admin, tokenLimit(nil, 1))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&s)
	assert.Equal(t, api.AiUsageLimitKindTokens, s.LimitKind)
	assert.True(t, s.LimitTokens.IsNull())
	assert.True(t, s.PriceInputPerMillion.IsNull())
	assert.Equal(t, 2, s.Version)

	// Each save is in the audit log with what changed (company settings: the values are kept).
	var changes []string
	rows, err := testutil.Current.Admin.Query(ctx,
		`SELECT changes::text FROM audit_log WHERE tenant_id = $1 AND action = 'ai_usage.settings_saved' ORDER BY id`, tenant.ID)
	require.NoError(t, err)
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		changes = append(changes, c)
	}
	require.NoError(t, rows.Err())
	require.Len(t, changes, 2)
	assert.JSONEq(t, `{"limitKind": {"from": "tokens", "to": "money"}, "limitTokens": {"from": null, "to": 3000},
		"limitMoney": {"from": null, "to": "50.00"}, "currency": {"from": "USD", "to": "MYR"},
		"priceInputPerMillion": {"from": null, "to": "0.2700"}, "priceOutputPerMillion": {"from": null, "to": "1.1000"},
		"warnPercent": {"from": 80, "to": 90}}`, changes[0])
	assert.JSONEq(t, `{"limitKind": {"from": "money", "to": "tokens"}, "limitTokens": {"from": 3000, "to": null},
		"limitMoney": {"from": "50.00", "to": null}, "currency": {"from": "MYR", "to": "USD"},
		"priceInputPerMillion": {"from": "0.2700", "to": null}, "priceOutputPerMillion": {"from": "1.1000", "to": null},
		"warnPercent": {"from": 90, "to": 80}}`, changes[1])

	// Another company's settings are its own.
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	assert.Equal(t, 0, a.aiUsageSettings(other.Users[api.RoleOwner].Token).Version)
}
