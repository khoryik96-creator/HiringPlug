package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

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
	year := time.Now().Year()
	day := func(y int, md string) string { return fmt.Sprintf("%d-%s", y, md) }
	place(maju.Id, "Accountant", day(year-1, "06-01"), "10000")
	place(maju.Id, "Buyer", day(year, "02-01"), "12000")
	place(maju.Id, "Clerk", day(year, "09-01"), "8000")
	place(other.Id, "Driver", day(year, "03-01"), "99000")

	billing := func(token, companyID string) (int, api.CompanyBilling) {
		res := a.request(http.MethodGet, "/v1/companies/"+companyID+"/billing", token, nil)
		var b api.CompanyBilling
		if res.Status == http.StatusOK {
			res.JSON(&b)
		}
		return res.Status, b
	}
	total := func(totals []api.FeeTotal) string {
		if len(totals) == 0 {
			return "none"
		}
		require.Len(t, totals, 1)
		return totals[0].Total
	}
	_, b := billing(token, maju.Id)
	assert.Equal(t, year, b.Year, "this year, in the agency's time zone")
	assert.Equal(t, "30000.00", total(b.AllTime), "every job's placements, other clients' left out")
	assert.Equal(t, "20000.00", total(b.ThisYear))
	_, none := billing(token, quiet.Id)
	assert.Equal(t, "none", total(none.AllTime), "no jobs, nothing billed")

	status, _ := billing(tenant.Users[api.RoleHiringManager].Token, maju.Id)
	assert.Equal(t, http.StatusForbidden, status, "fees only for those who may see them")
}

// The fee totals count every placement that matches, not only the 500 the
// list shows.
func TestPlacementTotalsCountEveryPlacement(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter]
	company := a.createCompany(recruiter.Token, map[string]any{"name": "Busy Client"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Picker"})

	ctx := context.Background()
	tx, err := testutil.Current.App.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true), set_config('app.user_id', $2, true),
		set_config('app.permissions', 'placements.fees:read,placements.fees:update', true)`, tenant.ID, recruiter.ID)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		WITH c AS (INSERT INTO candidate (tenant_id, first_name, created_by)
		           SELECT app_current_tenant(), 'Picker ' || g, app_current_user() FROM generate_series(1, 505) g RETURNING id),
		     a AS (INSERT INTO application (tenant_id, job_order_id, candidate_id, stage, created_by)
		           SELECT app_current_tenant(), $1, id, 'placed', app_current_user() FROM c RETURNING id),
		     p AS (INSERT INTO placement (tenant_id, application_id, start_date, guarantee_days, created_by)
		           SELECT app_current_tenant(), id, DATE '2026-03-01', 90, app_current_user() FROM a RETURNING id)
		INSERT INTO placement_money (placement_id, tenant_id, fee_amount, fee_currency, updated_by)
		SELECT id, app_current_tenant(), 1000, 'MYR', app_current_user() FROM p`, job.Id)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	list := a.placements(recruiter.Token, "")
	assert.Len(t, list.Items, 500, "the list stops at 500")
	assert.Equal(t, 505, list.Total, "and says how many there are, so it is never taken for all of them")
	y, m, d := time.Now().In(a.deps.Config.TimeZone).Date()
	assert.Equal(t, time.Date(y, m, d, 0, 0, 0, 0, time.UTC), list.Today.Time, "today in the agency's time zone")
	assert.Equal(t, 0, a.placements(recruiter.Token, "?startFrom=2027-01-01").Total)
	require.NotNil(t, list.FeeTotals)
	require.Len(t, *list.FeeTotals, 1)
	assert.Equal(t, "505000.00", (*list.FeeTotals)[0].Total, "the totals count all 505")

	var billing api.CompanyBilling
	a.request(http.MethodGet, "/v1/companies/"+company.Id+"/billing", recruiter.Token, nil).JSON(&billing)
	require.Len(t, billing.AllTime, 1)
	assert.Equal(t, "505000.00", billing.AllTime[0].Total)
}
