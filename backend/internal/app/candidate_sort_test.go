package app_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The candidate list sorts by name, notice period, or current or expected
// salary (monthly), across the whole agency with pages, and the advanced
// search sorts the same way. Salary sorts follow salary access.
func TestCandidateListSorts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleSourcer)
	token := tenant.Users[api.RoleOwner].Token
	type person struct {
		name            string
		notice          any
		current, expect string
		period          string
	}
	for _, p := range []person{
		{"Zara", map[string]any{"value": 1, "unit": "month"}, "9000", "10000", "month"},
		{"Aina", map[string]any{"value": 2, "unit": "week"}, "120000", "150000", "year"}, // 10,000 / 12,500 a month
		{"Mei", nil, "", "", ""},
		{"Badrul", map[string]any{"value": 3, "unit": "month"}, "15000", "18000", "month"},
	} {
		c := a.createCandidate(token, map[string]any{"firstName": p.name, "noticePeriod": p.notice})
		if p.current != "" {
			r := a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", token, map[string]any{"expectedVersion": 0,
				"currentSalary":  map[string]any{"amount": p.current, "currency": "MYR", "period": p.period},
				"expectedSalary": map[string]any{"min": p.expect, "currency": "MYR", "period": p.period}})
			require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
		}
	}
	names := func(path, who string) ([]string, *int) {
		r := a.request(http.MethodGet, path, who, nil)
		require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
		var page api.CandidatePage
		r.JSON(&page)
		out := []string{}
		for _, c := range page.Items {
			out = append(out, c.FullName)
		}
		next, _ := page.NextOffset.Get()
		if !page.NextOffset.IsSpecified() || page.NextOffset.IsNull() {
			return out, nil
		}
		return out, &next
	}
	got, _ := names("/v1/candidates?sort=name", token)
	assert.Equal(t, []string{"Aina", "Badrul", "Mei", "Zara"}, got)
	got, _ = names("/v1/candidates?sort=notice", token)
	assert.Equal(t, []string{"Aina", "Zara", "Badrul", "Mei"}, got, "shortest first, unknown last")
	got, _ = names("/v1/candidates?sort=current_salary_desc", token)
	assert.Equal(t, []string{"Badrul", "Aina", "Zara", "Mei"}, got, "monthly equivalents: Aina's 120k a year is 10k a month")
	got, _ = names("/v1/candidates?sort=expected_salary_asc", token)
	assert.Equal(t, []string{"Zara", "Aina", "Badrul", "Mei"}, got)
	got, _ = names("/v1/candidates?sort=name_desc", token)
	assert.Equal(t, []string{"Zara", "Mei", "Badrul", "Aina"}, got)
	got, _ = names("/v1/candidates?sort=notice_desc", token)
	assert.Equal(t, []string{"Badrul", "Zara", "Aina", "Mei"}, got, "longest first, unknown still last")

	// Accents do not change the order, and another currency is never mixed in with ringgit.
	elise := a.createCandidate(token, map[string]any{"firstName": "Élise"})
	r := a.request(http.MethodPut, "/v1/candidates/"+elise.Id+"/compensation", token, map[string]any{"expectedVersion": 0,
		"currentSalary": map[string]any{"amount": "20000", "currency": "SGD", "period": "month"}, "expectedSalary": nil})
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	got, _ = names("/v1/candidates?sort=name", token)
	assert.Equal(t, []string{"Aina", "Badrul", "Élise", "Mei", "Zara"}, got)
	got, _ = names("/v1/candidates?sort=current_salary_desc", token)
	assert.Equal(t, []string{"Badrul", "Aina", "Zara", "Élise", "Mei"}, got, "ringgit first, then SGD on its own")

	// Pages of a sorted list continue with nextOffset.
	first, next := names("/v1/candidates?sort=name&limit=3", token)
	assert.Equal(t, []string{"Aina", "Badrul", "Élise"}, first)
	require.NotNil(t, next)
	rest, more := names(fmt.Sprintf("/v1/candidates?sort=name&limit=3&offset=%d", *next), token)
	assert.Equal(t, []string{"Mei", "Zara"}, rest)
	assert.Nil(t, more)

	// Without salary access a salary sort is just newest first (nothing about salaries leaks).
	got, _ = names("/v1/candidates?sort=current_salary_desc", tenant.Users[api.RoleSourcer].Token)
	assert.Equal(t, []string{"Élise", "Badrul", "Mei", "Aina", "Zara"}, got)

	// The advanced search sorts too.
	r = a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"sort": "notice", "noticeMaxDays": 90})
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var found api.CandidateSearchPage
	r.JSON(&found)
	got = nil
	for _, h := range found.Items {
		got = append(got, h.Candidate.FullName)
	}
	assert.Equal(t, []string{"Aina", "Zara", "Badrul"}, got)
}
