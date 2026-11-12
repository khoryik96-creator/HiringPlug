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

// The placement sheet: title, client and job type start from the job and can
// be changed on the placement; PO number and end date are optional.
func TestPlacementSheet(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})
	place := func(job map[string]any, body map[string]any) api.Placement {
		job["companyId"] = company.Id
		j := a.createJob(token, job)
		app := a.addToJob(token, j.Id, a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"}).Id)
		return a.placement(http.MethodPost, "/v1/applications/"+app.Id+"/placement", token, body, http.StatusCreated)
	}

	p := place(map[string]any{"title": "Finance Manager", "workType": "contract"}, map[string]any{"startDate": "2026-12-01"})
	assert.Equal(t, "Finance Manager", p.JobTitle, "from the job")
	assert.Equal(t, "Maju Foods Berhad", p.CompanyName, "from the job's client")
	assert.Equal(t, api.PlacementJobTypeContract, p.JobType, "a contract job is a contract placement")
	assert.False(t, p.PoNumber.IsSpecified() && !p.PoNumber.IsNull())
	assert.True(t, p.EndDate.IsNull())

	perm := place(map[string]any{"title": "Accountant", "workType": "full_time"}, map[string]any{
		"startDate": "2026-12-01", "jobTitle": " Senior Accountant ", "poNumber": "PO-88123", "endDate": "2027-11-30",
	})
	assert.Equal(t, "Senior Accountant", perm.JobTitle, "what was entered, tidied")
	assert.Equal(t, api.PlacementJobTypePermanent, perm.JobType)
	po, _ := perm.PoNumber.Get()
	assert.Equal(t, "PO-88123", po)
	end, _ := perm.EndDate.Get()
	assert.Equal(t, "2027-11-30", end.String())

	t.Run("editable on the placement", func(t *testing.T) {
		path := "/v1/placements/" + p.Id
		res := a.request(http.MethodPatch, path, token, map[string]any{"version": p.Version, "endDate": "2026-11-01"})
		assert.Equal(t, http.StatusBadRequest, res.Status)
		assert.Contains(t, string(res.Body), "endDate", "an end before the start")
		p = a.placement(http.MethodPatch, path, token, map[string]any{
			"version": p.Version, "jobTitle": "Finance Lead", "companyName": "Maju Foods (M) Sdn Bhd", "jobType": "permanent",
			"poNumber": "PO-1", "endDate": "2027-05-31",
		}, http.StatusOK)
		assert.Equal(t, "Finance Lead", p.JobTitle)
		assert.Equal(t, "Maju Foods (M) Sdn Bhd", p.CompanyName)
		assert.Equal(t, api.PlacementJobTypePermanent, p.JobType)
		assert.Equal(t, "Finance Lead", a.placements(token, "?applicationId="+p.ApplicationId).Items[0].JobTitle, "and in the list")

		p = a.placement(http.MethodPatch, path, token, map[string]any{"version": p.Version, "jobTitle": "", "poNumber": nil, "endDate": nil}, http.StatusOK)
		assert.Equal(t, "Finance Manager", p.JobTitle, "cleared: the job's title again")
		assert.True(t, p.PoNumber.IsNull())
		assert.True(t, p.EndDate.IsNull())
		assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPatch, path, token, map[string]any{"version": p.Version, "jobType": "temporary"}).Status)

		var changes string
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT changes::text FROM audit_log WHERE entity_id = $1 AND action = 'placement.updated' ORDER BY id LIMIT 1`, p.Id).Scan(&changes))
		assert.Contains(t, changes, "PO-1")
	})
}
