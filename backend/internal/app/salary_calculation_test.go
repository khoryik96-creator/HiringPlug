package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestSalaryCalculationRulesAndProtectedDescriptions(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter, api.RoleSourcer)
	admin, recruiter, sourcer := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	path := "/v1/settings/salary-calculation"
	var rules api.SalaryCalculationRules
	a.request(http.MethodGet, path, recruiter, nil).JSON(&rules)
	assert.True(t, rules.Allowances)
	assert.True(t, rules.Bonus)
	changed := rules
	changed.Bonus = false
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, path, recruiter, changed).Status)
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, path, admin, changed).Status)
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	var isolated api.SalaryCalculationRules
	a.request(http.MethodGet, path, other, nil).JSON(&isolated)
	assert.True(t, isolated.Bonus, "settings are scoped to their company")
	current := "16k monthly + 400 allowance + 13.5th month bonus + 3 months performance + 16% EPF"
	body := map[string]any{"current": current, "expected": "10% lateral", "rules": rules}
	res := a.request(http.MethodPost, "/v1/salary-calculations", recruiter, body)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var calculation api.SalaryCalculation
	res.JSON(&calculation)
	assert.Equal(t, "18400.00", calculation.Monthly)
	require.NotNil(t, calculation.ExpectedMin)
	assert.Equal(t, "20240.00", *calculation.ExpectedMin)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/salary-calculations", sourcer, body).Status)
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Synthetic Salary Example"})
	call := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/screening-calls", recruiter, map[string]any{
		"version": c.Version, "compensation": map[string]any{"expectedVersion": 0,
			"currentSalary":  map[string]any{"amount": calculation.Monthly, "currency": "MYR", "period": "month", "description": current},
			"expectedSalary": map[string]any{"min": *calculation.ExpectedMin, "currency": "MYR", "period": "month", "description": "10% lateral"}},
	})
	require.Equal(t, http.StatusOK, call.Status, "%s", call.Body)
	var saved api.ScreeningCallResult
	call.JSON(&saved)
	comp := saved.Candidate.Compensation.MustGet()
	require.NotNil(t, comp.CurrentSalary.MustGet().Description)
	assert.Equal(t, current, *comp.CurrentSalary.MustGet().Description)
	assert.Equal(t, "18400.00", comp.CurrentSalary.MustGet().Amount)
	read := a.request(http.MethodGet, "/v1/candidates/"+c.Id, sourcer, nil)
	assert.NotContains(t, string(read.Body), "13.5th")
	assert.NotContains(t, string(read.Body), "10% lateral")
	// Clearing salary removes its description too; a later numeric-only edit
	// must never resurrect a package description from an old screening call.
	compPath := "/v1/candidates/" + c.Id + "/compensation"
	cleared := a.request(http.MethodPut, compPath, recruiter, map[string]any{"expectedVersion": 1, "currentSalary": nil, "expectedSalary": nil})
	require.Equal(t, http.StatusOK, cleared.Status, "%s", cleared.Body)
	renewed := a.request(http.MethodPut, compPath, recruiter, map[string]any{"expectedVersion": 2, "currentSalary": map[string]any{"amount": "20000", "currency": "MYR", "period": "month"}, "expectedSalary": nil})
	require.Equal(t, http.StatusOK, renewed.Status, "%s", renewed.Body)
	var fresh api.Compensation
	renewed.JSON(&fresh)
	assert.Nil(t, fresh.CurrentSalary.MustGet().Description)

}

func TestSalaryCalculationReviewFlagsAndCurrencies(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	for _, tc := range []struct {
		current, expected, want string
		review, has             bool
		currency                string
	}{
		{"16k", "1000%", "", true, true, ""},
		{"MYR 16k + USD 400 fixed allowance", "", "", true, true, ""},
		{"-16k + 400 allowance", "", "", true, false, ""},
		{"16k", "SGD 10% lateral", "", true, true, ""},
		{"SGD 16k", "maintain", "16000.00", false, true, "SGD"},
		{"16k", "15k + 20% annual bonus", "15000.00", false, true, ""},
		{"16k + 2400 yearly fixed allowance", "10%", "17820.00", false, true, "MYR"},
		{"", "22k", "22000.00", false, false, ""},
		{"0", "lateral", "0.00", false, true, "MYR"},
	} {
		res := a.request(http.MethodPost, "/v1/salary-calculations", token, map[string]any{"current": tc.current, "expected": tc.expected, "currentCurrency": "MYR", "rules": api.SalaryCalculationRules{Allowances: true, Bonus: true}})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var out api.SalaryCalculation
		res.JSON(&out)
		assert.Equal(t, tc.review, out.ReviewRequired, tc.current+" / "+tc.expected)
		assert.Equal(t, tc.has, out.HasCurrent)
		if tc.want == "" {
			assert.Nil(t, out.ExpectedMin)
		} else {
			require.NotNil(t, out.ExpectedMin)
			assert.Equal(t, tc.want, *out.ExpectedMin)
		}
		if tc.currency != "" {
			require.NotNil(t, out.ExpectedCurrency)
			assert.Equal(t, tc.currency, *out.ExpectedCurrency)
		}
	}
}

func TestScreeningCallSavesFixedFiguresWithUnconfirmedBonus(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	current := "9.6k + 2k allowance + 13.5 month bonus + 3 months bonus + 200 allowance + 13% EPF"
	res := a.request(http.MethodPost, "/v1/salary-calculations", token, map[string]any{
		"current": current, "expected": "14-18%", "currentCurrency": "MYR",
		"rules": api.SalaryCalculationRules{Allowances: true, Bonus: true},
	})
	require.Equal(t, http.StatusOK, res.Status)
	var calculation api.SalaryCalculation
	res.JSON(&calculation)
	require.False(t, calculation.ReviewRequired)
	require.Equal(t, "13000.00", calculation.Monthly)
	require.NotNil(t, calculation.ExpectedMin)
	require.NotNil(t, calculation.ExpectedMax)
	assert.Equal(t, "14820.00", *calculation.ExpectedMin)
	assert.Equal(t, "15340.00", *calculation.ExpectedMax)
	c := a.createCandidate(token, map[string]any{"firstName": "Synthetic Bonus Review"})
	call := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/screening-calls", token, map[string]any{
		"version": c.Version, "compensation": map[string]any{"expectedVersion": 0,
			"currentSalary":  map[string]any{"amount": calculation.Monthly, "currency": "MYR", "period": "month", "description": current},
			"expectedSalary": map[string]any{"min": *calculation.ExpectedMin, "max": *calculation.ExpectedMax, "currency": "MYR", "period": "month", "description": "14-18%"}},
	})
	require.Equal(t, http.StatusOK, call.Status)
	var saved api.ScreeningCallResult
	call.JSON(&saved)
	comp := saved.Candidate.Compensation.MustGet()
	assert.Equal(t, "13000.00", comp.CurrentSalary.MustGet().Amount)
	require.NotNil(t, comp.CurrentSalary.MustGet().Description)
	assert.Equal(t, current, *comp.CurrentSalary.MustGet().Description)
}

func TestScreeningRulesDoNotRequireSettingsAccess(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleSourcer)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": []api.Permission{api.PermissionCandidatesCompensationUpdate}, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	rules := api.SalaryCalculationRules{Allowances: true, Bonus: false, Cents: true}
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/settings/salary-calculation", owner, rules).Status)
	require.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/settings/salary-calculation", recruiter, nil).Status)
	res := a.request(http.MethodGet, "/v1/salary-calculation-rules", recruiter, nil)
	require.Equal(t, http.StatusOK, res.Status)
	var got api.SalaryCalculationRules
	res.JSON(&got)
	assert.Equal(t, rules, got)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/salary-calculation-rules", tenant.Users[api.RoleSourcer].Token, nil).Status)
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	var isolated api.SalaryCalculationRules
	a.request(http.MethodGet, "/v1/salary-calculation-rules", other, nil).JSON(&isolated)
	assert.True(t, isolated.Bonus)
}
