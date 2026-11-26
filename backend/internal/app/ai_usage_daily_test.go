package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// From the owner's post-migration report (P1): besides the month's limit, a
// company caps the AI a day (all features together) and gives a feature its
// own monthly limit; whichever is reached first stops new AI work.
func TestADailyLimitAndAFeaturesOwnLimit(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance) // a month's allowance far above these limits
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, token := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token

	body := tokenLimit(nil, 0)
	body["dailyTokens"] = 5000
	body["featureTokens"] = map[string]any{"cv_reading": 1000}
	res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner, body)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var saved api.AiUsageSettings
	res.JSON(&saved)
	assert.Equal(t, int64(5000), saved.DailyTokens.MustGet())
	assert.Equal(t, map[string]int64{"cv_reading": 1000}, saved.FeatureTokens)

	// CV reading used its 1,000 this month: CVs are read by the rules alone,
	// while the assistant (no limit of its own) carries on.
	usedEarlier(t, tenant.ID, "cv_reading", "cv_earlier", 800, 200, time.Now())
	_, _, payload := readCV(t, a, aiConn, token, "SITI AISYAH")
	assert.False(t, *payload.UseAi, "CV reading's own limit is reached")
	c := a.createCandidate(token, map[string]any{"firstName": "Aina"})
	summary := map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"}
	assert.Equal(t, http.StatusAccepted, a.ask(token, summary).Status, "the assistant has its share")

	// Today's 5,000 used, all features together: the assistant stops too.
	usedEarlier(t, tenant.ID, "assistant", "ask_earlier", 3000, 1000, time.Now())
	res = a.ask(token, summary)
	assert.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeASSISTANTLIMITREACHED, res.Error().Error.Code)

	// A save without them keeps them; null and {} take them off.
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", owner, tokenLimit(nil, saved.Version))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&saved)
	assert.Equal(t, int64(5000), saved.DailyTokens.MustGet(), "kept")
	assert.Equal(t, map[string]int64{"cv_reading": 1000}, saved.FeatureTokens, "kept")
	body = tokenLimit(nil, saved.Version)
	body["dailyTokens"], body["featureTokens"] = nil, map[string]any{}
	res = a.request(http.MethodPut, "/v1/ai-usage/settings", owner, body)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	saved = api.AiUsageSettings{} // decoding into a map adds to it
	res.JSON(&saved)
	assert.True(t, saved.DailyTokens.IsNull())
	assert.Empty(t, saved.FeatureTokens)
	assert.Equal(t, http.StatusAccepted, a.ask(token, summary).Status, "no daily limit any more")

	// What is not a feature, or not a limit, is refused.
	body["featureTokens"] = map[string]any{"coffee": 10}
	body["version"] = saved.Version
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPut, "/v1/ai-usage/settings", owner, body).Status)
}
