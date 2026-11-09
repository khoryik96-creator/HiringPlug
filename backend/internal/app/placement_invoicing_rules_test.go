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

// The rules that keep invoices right: numbers never reused, the email queued
// once and moved with the hour, retried once the fee is recorded, a due date
// not before the invoice, and the email seen only by those who may see fees.
func TestInvoicingRules(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	ctx := context.Background()
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	contact := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Mei", "lastName": "Tan",
		"email": "mei." + tenant.ID + "@contoh.test"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "contactId": contact.Id, "title": "Accountant"})
	today := time.Now().In(a.deps.Config.TimeZone).Format(time.DateOnly)
	place := func(first string, money bool) api.Placement {
		appl := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": first, "lastName": "Tan"}).Id)
		body := map[string]any{"startDate": today}
		if money {
			body["money"] = map[string]any{"salary": map[string]any{"amount": "8000.00", "currency": "MYR", "period": "month"}, "feePercent": "20"}
		}
		return a.placement(http.MethodPost, "/v1/applications/"+appl.Id+"/placement", recruiter, body, http.StatusCreated)
	}
	invoicing := func(p api.Placement, auto bool) api.Placement {
		return a.placement(http.MethodPut, "/v1/placements/"+p.Id+"/invoicing", recruiter,
			map[string]any{"contactId": contact.Id, "paymentTermsDays": 30, "dueOn": nil, "autoSend": auto}, http.StatusOK)
	}
	pending := func(p api.Placement) int {
		var n int
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'placement.invoice'
			AND status = 'pending' AND payload->>'placementId' = $2`, tenant.ID, p.Id).Scan(&n))
		return n
	}
	runDue := func() {
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'placement.invoice' AND status = 'pending'`, tenant.ID)
		require.NoError(t, err)
		runCoreJobsUntilIdle(t, a, tenant.ID)
	}
	settings := func() api.InvoiceSettings {
		var s api.InvoiceSettings
		a.request(http.MethodGet, "/v1/invoice-settings", owner, nil).JSON(&s)
		return s
	}
	putSettings := func(s api.InvoiceSettings) *response {
		return a.request(http.MethodPut, "/v1/invoice-settings", owner, map[string]any{"version": s.Version, "issuer": s.Issuer,
			"bankDetails": s.BankDetails, "numberPrefix": s.NumberPrefix, "nextNumber": s.NextNumber, "taxLabel": s.TaxLabel,
			"taxPercent": s.TaxPercent, "lineText": s.LineText, "footer": s.Footer, "emailSubject": s.EmailSubject,
			"emailBody": s.EmailBody, "sendHour": s.SendHour})
	}

	s := settings()
	s.SendHour = 0 // at midnight on the start date: due whatever the time of the test
	require.Equal(t, http.StatusOK, putSettings(s).Status)

	t.Run("saving the invoicing again queues the email once", func(t *testing.T) {
		p := place("Ali", true)
		invoicing(p, true)
		invoicing(p, true)
		invoicing(p, true)
		assert.Equal(t, 1, pending(p))
	})

	t.Run("an email that could not go out is queued again once the fee is recorded", func(t *testing.T) {
		p := invoicing(place("Badrul", false), true)
		runDue()
		var got api.PlacementList
		a.request(http.MethodGet, "/v1/placements?applicationId="+p.ApplicationId, recruiter, nil).JSON(&got)
		p = got.Items[0]
		assert.Equal(t, "Record the placement's fee first.", p.Invoicing.SendError.MustGet())
		assert.True(t, p.Invoicing.SendsAt.IsNull(), "it does not say it will go out")
		p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, recruiter, map[string]any{"version": p.Version,
			"money": map[string]any{"salary": map[string]any{"amount": "8000.00", "currency": "MYR", "period": "month"}, "feePercent": "20"}}, http.StatusOK)
		assert.True(t, p.Invoicing.SendError.IsNull())
		assert.Equal(t, 1, pending(p))
		runDue()
		a.request(http.MethodGet, "/v1/placements?applicationId="+p.ApplicationId, recruiter, nil).JSON(&got)
		assert.Equal(t, api.InvoiceStatusInvoiced, got.Items[0].InvoiceStatus)
	})

	t.Run("a template opened before an invoice went out cannot put the number back", func(t *testing.T) {
		opened := settings()
		p := invoicing(place("Chong", true), false)
		res := a.request(http.MethodPost, "/v1/placements/"+p.Id+"/invoice", recruiter, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		opened.Footer = "Thank you."
		assert.Equal(t, http.StatusConflict, putSettings(opened).Status)
	})

	t.Run("a number already on an invoice is not used again", func(t *testing.T) {
		s := settings()
		used := s.NextNumber - 1
		s.NextNumber = used // set back by hand
		require.Equal(t, http.StatusOK, putSettings(s).Status)
		p := invoicing(place("Devi", true), false)
		res := a.request(http.MethodPost, "/v1/placements/"+p.Id+"/invoice", recruiter, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&p)
		assert.NotEqual(t, "INV-"+padded(used), p.InvoiceNumber.MustGet())
		assert.Equal(t, "INV-"+padded(used+1), p.InvoiceNumber.MustGet())

		var key string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT storage_key FROM email_attachment WHERE tenant_id = $1 AND file_name = $2`,
			tenant.ID, "Invoice "+p.InvoiceNumber.MustGet()+".pdf").Scan(&key))
		assert.Equal(t, tenant.ID+"/eat_invoice_"+p.Id+"_inv_"+padded(used+1), key, "the same file again on a retry")
		var fees bool
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT fees_restricted FROM email WHERE tenant_id = $1 AND subject LIKE '%' || $2 || '%'`,
			tenant.ID, p.InvoiceNumber.MustGet()).Scan(&fees))
		assert.True(t, fees, "seen only by those who may see fees")
	})

	t.Run("a new sending hour moves invoices still to go out", func(t *testing.T) {
		later := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Eng", "lastName": "Tan"}).Id)
		p := a.placement(http.MethodPost, "/v1/applications/"+later.Id+"/placement", recruiter, map[string]any{
			"startDate": time.Now().AddDate(0, 0, 10).Format(time.DateOnly),
			"money":     map[string]any{"salary": map[string]any{"amount": "8000.00", "currency": "MYR", "period": "month"}, "feePercent": "20"},
		}, http.StatusCreated)
		invoicing(p, true)
		s := settings()
		s.SendHour = 14
		require.Equal(t, http.StatusOK, putSettings(s).Status)
		var at time.Time
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT max(run_after) FROM job WHERE tenant_id = $1 AND kind = 'placement.invoice'
			AND payload->>'placementId' = $2`, tenant.ID, p.Id).Scan(&at))
		assert.Equal(t, 14, at.In(a.deps.Config.TimeZone).Hour())

		// The job for the old hour runs first: it leaves the one for 14:00 alone.
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'placement.invoice'
			AND status = 'pending' AND payload->>'placementId' = $2 AND run_after < $3`, tenant.ID, p.Id, at)
		require.NoError(t, err)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		assert.Equal(t, 1, pending(p), "one job, for 14:00")
	})

	t.Run("a due date before the invoice is refused", func(t *testing.T) {
		p := place("Farah", true)
		res := a.request(http.MethodPut, "/v1/placements/"+p.Id+"/invoicing", recruiter,
			map[string]any{"contactId": contact.Id, "paymentTermsDays": 30, "dueOn": "2020-01-01", "autoSend": false})
		require.Equal(t, http.StatusBadRequest, res.Status)
		assert.Equal(t, []string{"dueOn"}, fieldNames(t, res.Error()))
	})
}

func padded(n int) string { return fmt.Sprintf("%04d", n) }
