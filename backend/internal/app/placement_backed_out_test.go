package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// applicationOn reads one application from its job's board.
func (a *testApp) applicationOn(token, jobID, id string) api.Application {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/jobs/"+jobID+"/applications", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list struct{ Items []api.Application }
	res.JSON(&list)
	for _, item := range list.Items {
		if item.Id == id {
			return item
		}
	}
	a.t.Fatalf("application %s is not on job %s", id, jobID)
	return api.Application{}
}

// A placed candidate who pulls out (or whose offer the client withdraws) is
// moved back to offer or to withdrawn: the placement sheet stays, marked
// backed out, and its fee no longer counts. Placing them again revives it.
func TestPlacementBackedOut(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	admin := tenant.Users[api.RoleAdmin]
	company := a.createCompany(admin.Token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(admin.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	app := a.addToJob(admin.Token, job.Id, a.createCandidate(admin.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"}).Id)
	path := "/v1/applications/" + app.Id + "/placement"
	body := map[string]any{
		"startDate": "2026-12-01", "poNumber": "PO-1",
		"money": map[string]any{"feeAmount": "20000.00", "feeCurrency": "MYR"},
	}
	p := a.placement(http.MethodPost, path, admin.Token, body, http.StatusCreated)
	require.Len(t, *a.placements(admin.Token, "").FeeTotals, 1, "the fee counts while it stands")

	placed := a.applicationOn(admin.Token, job.Id, app.Id)
	require.Equal(t, http.StatusOK, a.move(admin.Token, placed, map[string]any{"stage": "offer"}).Status)

	list := a.placements(admin.Token, "")
	require.Len(t, list.Items, 1, "the sheet stays")
	backed := list.Items[0]
	assert.Equal(t, api.PlacementStatusBackedOut, backed.Status)
	assert.Equal(t, p.Id, backed.Id)
	po, _ := backed.PoNumber.Get()
	assert.Equal(t, "PO-1", po, "as it was recorded")
	money, err := backed.Money.Get()
	require.NoError(t, err)
	fee, _ := money.FeeAmount.Get()
	assert.Equal(t, "20000.00", fee, "the fee agreed is still on the sheet")
	assert.Empty(t, *list.FeeTotals, "but it no longer counts as billed")
	billing := a.request(http.MethodGet, "/v1/companies/"+company.Id+"/billing", admin.Token, nil)
	require.Equal(t, http.StatusOK, billing.Status, "%s", billing.Body)
	assert.Empty(t, billing.Map()["allTime"], "nor in what the client was billed")
	res := a.request(http.MethodPost, "/v1/placements/"+p.Id+"/invoice", admin.Token, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "nothing to invoice: %s", res.Body)

	t.Run("placing them again makes the same sheet active, with what is recorded now", func(t *testing.T) {
		again := a.placement(http.MethodPost, path, admin.Token, map[string]any{"startDate": "2027-01-15"}, http.StatusCreated)
		assert.Equal(t, p.Id, again.Id, "one placement per application")
		assert.Equal(t, api.PlacementStatusActive, again.Status)
		assert.Equal(t, "2027-01-15", again.StartDate.String())
		money, err := again.Money.Get()
		require.NoError(t, err)
		fee, _ := money.FeeAmount.Get()
		assert.Equal(t, "20000.00", fee, "the fee stays unless given again")
		assert.Equal(t, "placed", string(a.applicationOn(admin.Token, job.Id, app.Id).Stage))
		assert.Len(t, *a.placements(admin.Token, "").FeeTotals, 1, "and counts again")
		assert.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodPost, path, admin.Token, body).Status, "not while it stands")
	})

	t.Run("withdrawn backs it out too; one that fell through is left as it is", func(t *testing.T) {
		placed := a.applicationOn(admin.Token, job.Id, app.Id)
		require.Equal(t, http.StatusOK, a.move(admin.Token, placed, map[string]any{"stage": "withdrawn", "reason": "accepted_other_offer"}).Status)
		assert.Equal(t, api.PlacementStatusBackedOut, a.placements(admin.Token, "").Items[0].Status)

		other := a.addToJob(admin.Token, job.Id, a.createCandidate(admin.Token, map[string]any{"firstName": "Tan", "lastName": "Wei"}).Id)
		fell := a.placement(http.MethodPost, "/v1/applications/"+other.Id+"/placement", admin.Token, map[string]any{"startDate": "2026-01-05"}, http.StatusCreated)
		fell = a.placement(http.MethodPatch, "/v1/placements/"+fell.Id, admin.Token,
			map[string]any{"version": fell.Version, "status": "fell_through", "leftOn": "2026-02-01"}, http.StatusOK)
		require.Equal(t, http.StatusOK, a.move(admin.Token, a.applicationOn(admin.Token, job.Id, other.Id), map[string]any{"stage": "offer"}).Status)
		for _, item := range a.placements(admin.Token, "").Items {
			if item.Id == fell.Id {
				assert.Equal(t, api.PlacementStatusFellThrough, item.Status)
			}
		}
	})

	t.Run("it can be set by hand on the sheet", func(t *testing.T) {
		third := a.addToJob(admin.Token, job.Id, a.createCandidate(admin.Token, map[string]any{"firstName": "Lim", "lastName": "Hui"}).Id)
		made := a.placement(http.MethodPost, "/v1/applications/"+third.Id+"/placement", admin.Token, map[string]any{"startDate": "2026-12-01"}, http.StatusCreated)
		made = a.placement(http.MethodPatch, "/v1/placements/"+made.Id, admin.Token, map[string]any{"version": made.Version, "status": "backed_out"}, http.StatusOK)
		assert.Equal(t, api.PlacementStatusBackedOut, made.Status)
	})
}
