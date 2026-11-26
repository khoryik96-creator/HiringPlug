package app_test

// The company's own monthly AI limit, set in Settings, stops every AI
// feature: the assistant and matching refuse, CV reading falls back to the
// rules.

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestTheCompanysOwnAILimitStopsEveryAIFeature(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, bigAllowance) // the server's default: 1,000,000 tokens a month
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, token := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token

	// 3,000 tokens a month for this company; 2,000 already used.
	res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner, tokenLimit(3000, 0))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	usedEarlier(t, tenant.ID, "assistant", "ask_earlier", 1500, 500, time.Now())

	// A CV read takes the last 1,000 (held while it is read).
	_, _, payload := readCV(t, a, aiConn, token, "SITI AISYAH")
	assert.True(t, *payload.UseAi, "1,000 tokens were left")

	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	company := a.createCompany(token, map[string]any{"name": "Contoh Sdn Bhd"}) // no AI industry pick now
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	appl := a.addToJob(token, job.Id, c.Id)
	summary := map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"}

	res = a.ask(token, summary)
	assert.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeASSISTANTLIMITREACHED, res.Error().Error.Code)
	assert.Equal(t, "The AI allowance (this month's, today's, or this feature's own in Settings › AI usage) is used or reserved by work in progress. Try again after it completes, tomorrow, or next month.",
		res.Error().Error.Message)
	res = a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": appl.Id})
	assert.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMATCHINGLIMITREACHED, res.Error().Error.Code)
	_, _, payload = readCV(t, a, aiConn, token, "AHMAD ALI")
	assert.False(t, *payload.UseAi, "read by the rules only")
	assert.Nil(t, payload.AiText, "nothing goes to the AI")

	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.Equal(t, api.AssistantStatus{Enabled: true, UsedTokens: 2000, MonthlyTokens: 3000, UsedPercent: 66}, status,
		"what was spent, of the company's own limit (work in progress aside)")

	t.Run("another company keeps the server's default", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		person := a.createCandidate(other, map[string]any{"firstName": "Lim"})
		res := a.ask(other, map[string]any{"subjectType": "candidate", "subjectId": person.Id, "kind": "summary"})
		assert.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	})

	t.Run("a limit in money", func(t *testing.T) {
		var set api.AiUsageSettings
		a.request(http.MethodGet, "/v1/ai-usage/settings", owner, nil).JSON(&set)
		// 1.00 at 100 a million tokens (both ways) buys 10,000 tokens: 8,000 are left.
		res := a.request(http.MethodPut, "/v1/ai-usage/settings", owner, map[string]any{"limitKind": "money", "limitTokens": nil,
			"limitMoney": "1.00", "currency": "USD", "priceInputPerMillion": "100", "priceOutputPerMillion": "100", "warnPercent": 80,
			"version": set.Version})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res = a.ask(token, summary)
		require.Equal(t, http.StatusAccepted, res.Status, "money left: %s", res.Body)
		usedEarlier(t, tenant.ID, "fit_score", "match_earlier", 7000, 0, time.Now()) // 9,000 spent, 2,000 held
		res = a.ask(token, summary)
		assert.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
		assert.Equal(t, api.ErrorCodeASSISTANTLIMITREACHED, res.Error().Error.Code)
		a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
		assert.Equal(t, api.AssistantStatus{Enabled: true, UsedTokens: 9000, MonthlyTokens: 0, UsedPercent: 90}, status,
			"a money limit has no token number: the share used says it")
	})
}
