package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestEmailSalaryPreferenceCleansWordingWithoutChangingStoredNotes(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleViewer)
	token := tenant.Users[api.RoleOwner].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Synthetic Salary Email"})
	raw := "16k + 400 alowance + perf bonus + epf13%"
	r := a.request("PUT", "/v1/candidates/"+c.Id+"/compensation", token, map[string]any{"expectedVersion": 0, "currentSalary": map[string]any{"amount": "16400", "currency": "MYR", "period": "month", "description": raw}, "expectedSalary": nil, "salaryNotes": "perf bonus is variable"})
	require.Equal(t, 200, r.Status)
	preview := func(auth string) api.EmailPreview {
		r := a.request("POST", "/v1/email-previews", auth, map[string]any{"candidateId": c.Id, "subject": "", "bodyHtml": "", "timeZone": "Asia/Kuala_Lumpur"})
		require.Equal(t, 200, r.Status, "%s", r.Body)
		var out api.EmailPreview
		r.JSON(&out)
		return out
	}
	assert.Contains(t, preview(token).FieldValues["[Current salary]"], "16,400")
	r = a.request("PUT", "/v1/settings/salary-calculation", token, map[string]any{"allowances": true, "bonus": true, "bonusOnAllowances": false, "cents": false, "emailSalaryDisplay": "breakdown"})
	require.Equal(t, 200, r.Status)
	out := preview(token)
	assert.Equal(t, "16k + 400 allowance + performance bonus + 13% EPF", out.FieldValues["[Current salary]"])
	assert.Equal(t, "performance bonus is variable", out.FieldValues["[Salary notes]"])
	var stored api.Candidate
	a.request("GET", "/v1/candidates/"+c.Id, token, nil).JSON(&stored)
	assert.Equal(t, raw, *stored.Compensation.MustGet().CurrentSalary.MustGet().Description)
	assert.Equal(t, "perf bonus is variable", stored.Compensation.MustGet().SalaryNotes.MustGet())
	r = a.request("PUT", "/v1/settings/salary-calculation", token, map[string]any{"allowances": true, "bonus": true, "bonusOnAllowances": false, "cents": false, "emailSalaryDisplay": "unknown"})
	assert.Equal(t, 400, r.Status)
}
