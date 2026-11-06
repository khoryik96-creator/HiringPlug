package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A job's market map: who in the database does the work, their pay for roles
// that may see it, the AI's market insights (which see counts, never pay or
// names), and the map as Word or PDF.
func TestMarketMap(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Senior Finance Manager"})

	pay := func(id, current, expected string) {
		res := a.request(http.MethodPut, "/v1/candidates/"+id+"/compensation", token, map[string]any{
			"expectedVersion": 0, "currentSalary": map[string]any{"amount": current, "currency": "MYR", "period": "month"},
			"expectedSalary": map[string]any{"min": expected, "max": nil, "currency": "MYR", "period": "month"},
		})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}
	siti := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "currentTitle": "Finance Manager",
		"currentCompany": "Nestlé Malaysia", "location": "Petaling Jaya", "residenceCountry": "MY", "availability": "looking",
		"workAuthorization": map[string]any{"status": "local"}, "noticePeriod": map[string]any{"value": 1, "unit": "month"}})
	ali := a.createCandidate(token, map[string]any{"firstName": "Ali", "currentTitle": "Head of Finance", "currentCompany": "Nestlé Malaysia",
		"availability": "passive", "workAuthorization": map[string]any{"status": "expat_eligible", "basis": "rp_t"},
		"noticePeriod": map[string]any{"value": 3, "unit": "month"}})
	mei := a.createCandidate(token, map[string]any{"firstName": "Mei", "currentTitle": "Finance Executive", "currentCompany": "Grab"})
	a.createCandidate(token, map[string]any{"firstName": "Raj", "currentTitle": "Software Engineer", "currentCompany": "Grab"})
	pay(siti.Id, "12000", "15000")
	pay(ali.Id, "20000", "24000")
	pay(mei.Id, "6000", "7500")

	get := func(token string) api.MarketMap {
		var m api.MarketMap
		res := a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/market-map", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&m)
		return m
	}

	t.Run("who does the work: counts, where they work and live, notice, pay", func(t *testing.T) {
		m := get(token)
		assert.Equal(t, "finance", m.SearchedFor, "level words left out")
		p := m.Pool
		assert.Equal(t, 3, p.Total, "the software engineer is not counted")
		assert.Equal(t, []api.MarketCount{{Key: "Nestlé Malaysia", Count: 2}, {Key: "Grab", Count: 1}}, p.Companies)
		assert.Contains(t, p.Availability, api.MarketCount{Key: "looking", Count: 1})
		assert.Contains(t, p.Availability, api.MarketCount{Key: "unknown", Count: 1})
		assert.Contains(t, p.WorkAuthorization, api.MarketCount{Key: "expat", Count: 1})
		assert.Equal(t, []api.MarketCount{{Key: "Petaling Jaya, Malaysia", Count: 1}}, p.Locations)
		assert.Equal(t, 60, p.MedianNoticeDays.MustGet())
		require.Len(t, p.Salary, 1)
		cur := p.Salary[0].Current.MustGet()
		assert.Equal(t, api.MarketRange{Count: 3, Low: "9000", Median: "12000", High: "16000"}, cur)
		assert.Equal(t, "15000", p.Salary[0].Expected.MustGet().Median)
		assert.False(t, p.SalaryRestricted)
		assert.True(t, m.Insights.IsNull())
	})

	t.Run("no pay for roles that may not see salaries; no map for those without access", func(t *testing.T) {
		m := get(tenant.Users[api.RoleSourcer].Token)
		assert.Equal(t, 3, m.Pool.Total)
		assert.Empty(t, m.Pool.Salary)
		assert.True(t, m.Pool.SalaryRestricted)
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/market-map", tenant.Users[api.RoleHiringManager].Token, nil).Status)
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/market-map", other, nil).Status)
	})

	t.Run("AI insights see the counts, never pay or names", func(t *testing.T) {
		aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
		require.NoError(t, err)
		defer func() { _ = aiConn.Close(ctx) }()
		assert.Equal(t, http.StatusBadRequest, a.ask(token, map[string]any{"subjectType": "candidate", "subjectId": siti.Id, "kind": "market_map"}).Status)
		res := a.ask(token, map[string]any{"subjectType": "job", "subjectId": job.Id, "kind": "market_map"})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var asked api.AssistantAnswer
		res.JSON(&asked)
		sent := aiAnswers(t, a, aiConn, asked.Id, api.AssistantAnsweredJob{
			Answer: nullable.NewNullableWithValue("Talent supply\n- Tight in Klang Valley."), Error: nullable.NewNullNullable[string](),
			Model: nullable.NewNullableWithValue("deepseek-chat"), InputTokens: 100, OutputTokens: 50,
		})
		var all strings.Builder
		for _, s := range sent.Sections {
			all.WriteString(s.Title + "\n" + s.Text + "\n")
		}
		assert.Contains(t, all.String(), "3 candidates have \"finance\" in their current title.")
		assert.Contains(t, all.String(), "Where they work now: Nestlé Malaysia 2, Grab 1")
		for _, secret := range []string{"12000", "12,000", "15000", "Siti", "Aisyah"} {
			assert.NotContains(t, all.String(), secret)
		}
		in := get(token).Insights.MustGet()
		assert.Equal(t, api.AssistantAnswerStatusDone, in.Status)
		assert.Equal(t, "Talent supply\n- Tight in Klang Valley.", in.Text.MustGet())
	})

	t.Run("exported as Word and PDF", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/market-map/export?format=docx", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, "PK", string(res.Body[:2]))
		assert.Contains(t, res.Header.Get("Content-Disposition"), "Market map - Senior Finance Manager.docx")
		res = a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/market-map/export?format=pdf", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, "%PDF", string(res.Body[:4]))
	})
}
