package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A company's billing: the fees of placements on its jobs (every job, open
// or closed), for a year or all time; other companies' placements stay out.
func TestCompanyBilling(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager)
	token := tenant.Users[api.RoleRecruiter].Token
	maju := a.createCompany(token, map[string]any{"name": "Maju Berhad"})
	other := a.createCompany(token, map[string]any{"name": "Other Sdn Bhd"})
	quiet := a.createCompany(token, map[string]any{"name": "No Jobs Yet"})
	place := func(companyID, title, start, fee string) {
		job := a.createJob(token, map[string]any{"companyId": companyID, "title": title})
		app := a.addToJob(token, job.Id, a.createCandidate(token, map[string]any{"firstName": title}).Id)
		a.placement(http.MethodPost, "/v1/applications/"+app.Id+"/placement", token, map[string]any{
			"startDate": start, "money": map[string]any{"feeAmount": fee, "feeCurrency": "MYR"},
		}, http.StatusCreated)
	}
	place(maju.Id, "Accountant", "2025-06-01", "10000")
	place(maju.Id, "Buyer", "2026-02-01", "12000")
	place(maju.Id, "Clerk", "2026-09-01", "8000")
	place(other.Id, "Driver", "2026-03-01", "99000")

	total := func(list api.PlacementList) string {
		require.NotNil(t, list.FeeTotals)
		if len(*list.FeeTotals) == 0 {
			return "none"
		}
		return (*list.FeeTotals)[0].Total
	}
	allTime := a.placements(token, "?companyId="+maju.Id)
	assert.Len(t, allTime.Items, 3)
	assert.Equal(t, "30000.00", total(allTime))
	thisYear := a.placements(token, "?companyId="+maju.Id+"&startFrom=2026-01-01&startTo=2026-12-31")
	assert.Equal(t, "20000.00", total(thisYear))
	assert.Empty(t, a.placements(token, "?companyId="+quiet.Id).Items, "no jobs, no placements")
	assert.Len(t, a.placements(token, "").Items, 4, "without a company: every placement")

	hidden := a.placements(tenant.Users[api.RoleHiringManager].Token, "?companyId="+maju.Id)
	assert.Len(t, hidden.Items, 3)
	assert.Nil(t, hidden.FeeTotals, "fees only for those who may see them")
}
