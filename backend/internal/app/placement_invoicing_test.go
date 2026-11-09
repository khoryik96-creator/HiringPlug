package app_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A client's terms of business fill in a placement; the placement sheet
// names who receives the invoice and when it is due; on the start date the
// invoice, made from the agency's template, is emailed to them.
func TestPlacementInvoicing(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	owner, recruiter, viewer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleViewer].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Maju Foods Berhad"})
	hiring := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman",
		"email": "aisyah." + tenant.ID + "@majufoods.test"})
	finance := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Ken", "lastName": "Lim",
		"email": "ken." + tenant.ID + "@majufoods.test"})
	other := a.createContact(recruiter, map[string]any{"companyId": a.createCompany(recruiter, map[string]any{"name": "Grab"}).Id, "firstName": "Wei"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "contactId": hiring.Id, "title": "Finance Manager"})
	appl := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah"}).Id)

	terms := "/v1/companies/" + company.Id + "/terms"
	t.Run("a client's terms of business", func(t *testing.T) {
		var got api.CompanyTerms
		a.request(http.MethodGet, terms, viewer, nil).JSON(&got)
		assert.Equal(t, 0, got.Version)
		bad := a.request(http.MethodPut, terms, recruiter, map[string]any{"version": 0, "feePercent": "0", "paymentTermsDays": 30,
			"guaranteeDays": 60, "signedOn": "2026-01-15", "notes": nil})
		require.Equal(t, http.StatusBadRequest, bad.Status)
		assert.Equal(t, []string{"feePercent"}, fieldNames(t, bad.Error()))
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, terms, viewer, map[string]any{"version": 0, "feePercent": "18",
			"paymentTermsDays": 30, "guaranteeDays": 60, "signedOn": nil, "notes": nil}).Status)
		res := a.request(http.MethodPut, terms, recruiter, map[string]any{"version": 0, "feePercent": "18.00", "paymentTermsDays": 30,
			"guaranteeDays": 60, "signedOn": "2026-01-15", "notes": " Replacement once within the guarantee. "})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&got)
		assert.Equal(t, "18", string(got.FeePercent.MustGet()))
		assert.Equal(t, "Replacement once within the guarantee.", got.Notes.MustGet())
		assert.Equal(t, 1, got.Version)
	})

	var defaults api.PlacementDefaults
	a.request(http.MethodGet, "/v1/applications/"+appl.Id+"/placement-defaults", recruiter, nil).JSON(&defaults)
	assert.True(t, defaults.TermsSet)
	assert.Equal(t, "18", string(defaults.FeePercent.MustGet()))
	assert.Equal(t, 30, defaults.PaymentTermsDays)
	assert.Equal(t, 60, defaults.GuaranteeDays)
	assert.Equal(t, hiring.Id, defaults.ContactId.MustGet(), "the job's hiring contact")

	today := time.Now().In(a.deps.Config.TimeZone).Format(time.DateOnly)
	p := a.placement(http.MethodPost, "/v1/applications/"+appl.Id+"/placement", recruiter, map[string]any{
		"startDate": today, "guaranteeDays": 60,
		"money": map[string]any{"salary": map[string]any{"amount": "10000.00", "currency": "MYR", "period": "month"}, "feePercent": "18"},
	}, http.StatusCreated)
	assert.Equal(t, company.Id, p.CompanyId)
	assert.Equal(t, hiring.Id, p.Invoicing.ContactId.MustGet(), "invoiced to the hiring contact unless chosen")
	assert.Equal(t, 30, p.Invoicing.PaymentTermsDays, "the client's payment terms")
	assert.False(t, p.Invoicing.AutoSend, "not emailed until someone says so")
	start, _ := time.Parse(time.DateOnly, today)
	assert.Equal(t, start.AddDate(0, 0, 30).Format(time.DateOnly), p.Invoicing.DueOn.String(), "the start date plus the terms")

	invoicing := "/v1/placements/" + p.Id + "/invoicing"
	t.Run("who receives it and when it is due", func(t *testing.T) {
		res := a.request(http.MethodPut, invoicing, recruiter, map[string]any{"contactId": other.Id, "paymentTermsDays": 45, "dueOn": nil, "autoSend": true})
		require.Equal(t, http.StatusBadRequest, res.Status)
		assert.Equal(t, []string{"contactId"}, fieldNames(t, res.Error()), "only the client's own contacts")
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, invoicing, viewer, map[string]any{"contactId": finance.Id,
			"paymentTermsDays": 45, "dueOn": nil, "autoSend": true}).Status)

		p = a.placement(http.MethodPut, invoicing, recruiter, map[string]any{"contactId": finance.Id, "paymentTermsDays": 45, "dueOn": "2027-02-01", "autoSend": false}, http.StatusOK)
		assert.Equal(t, "2027-02-01", p.Invoicing.DueOn.String())
		assert.True(t, p.Invoicing.DueOnSet)
		p = a.placement(http.MethodPut, invoicing, recruiter, map[string]any{"contactId": finance.Id, "paymentTermsDays": 45, "dueOn": nil, "autoSend": true}, http.StatusOK)
		assert.Equal(t, start.AddDate(0, 0, 45).Format(time.DateOnly), p.Invoicing.DueOn.String(), "back to the start date plus the terms")
		assert.Equal(t, "ken."+tenant.ID+"@majufoods.test", p.Invoicing.ContactEmail.MustGet())
		assert.NotNil(t, p.Invoicing.SendsAt.MustGet(), "to be emailed on the start date")
	})

	t.Run("the agency's template", func(t *testing.T) {
		var s api.InvoiceSettings
		a.request(http.MethodGet, "/v1/invoice-settings", recruiter, nil).JSON(&s)
		assert.Equal(t, 0, s.Version)
		assert.Contains(t, s.LineText, "{{candidate.name}}")
		s.TaxPercent, s.NumberPrefix, s.NextNumber = "8", "HYP-", 42
		s.Issuer = "Hyppies Sdn Bhd\nLevel 5, Menara KL\nSST No. W10-1234"
		s.BankDetails = "Maybank 5140 1234 5678 (Hyppies Sdn Bhd)"
		s.EmailSubject = "Invoice {{invoice.number}} for {{candidate.name}}"
		body := map[string]any{"version": s.Version, "issuer": s.Issuer, "bankDetails": s.BankDetails, "numberPrefix": s.NumberPrefix,
			"nextNumber": s.NextNumber, "taxLabel": s.TaxLabel, "taxPercent": s.TaxPercent, "lineText": s.LineText, "footer": s.Footer,
			"emailSubject": s.EmailSubject, "emailBody": s.EmailBody, "sendHour": s.SendHour}
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/invoice-settings", recruiter, body).Status)
		body["emailBody"] = "Hi {{contact.nickname}}"
		res := a.request(http.MethodPut, "/v1/invoice-settings", owner, body)
		require.Equal(t, http.StatusBadRequest, res.Status, "an unknown field is refused")
		body["emailBody"] = s.EmailBody
		res = a.request(http.MethodPut, "/v1/invoice-settings", owner, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	})

	t.Run("a preview before it is sent", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/placements/"+p.Id+"/invoice/file?format=docx", recruiter, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		doc := docxText(t, res.Body)
		for _, want := range []string{"INVOICE", "Hyppies Sdn Bhd", "(numbered when sent)", "Maju Foods Berhad", "Ken Lim",
			"Recruitment fee: Siti Aisyah, Finance Manager", "18% of the annual salary of MYR 120,000.00",
			"MYR 21,600.00", "SST 8%", "MYR 1,728.00", "MYR 23,328.00", "Maybank 5140 1234 5678", "within 45 days"} {
			assert.Contains(t, doc, want)
		}
		pdf := a.request(http.MethodGet, "/v1/placements/"+p.Id+"/invoice/file?format=pdf", recruiter, nil)
		require.Equal(t, http.StatusOK, pdf.Status)
		assert.True(t, bytes.HasPrefix(pdf.Body, []byte("%PDF-")))
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/placements/"+p.Id+"/invoice/file?format=pdf", viewer, nil).Status,
			"it shows the fee")
	})

	t.Run("on the start date it is emailed, numbered, and the placement is invoiced", func(t *testing.T) {
		_, err := testutil.Current.Admin.Exec(context.Background(),
			`UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'placement.invoice'`, tenant.ID)
		require.NoError(t, err)
		runCoreJobsUntilIdle(t, a, tenant.ID)

		p = a.placements(recruiter, "?applicationId="+appl.Id).Items[0]
		assert.Equal(t, api.InvoiceStatusInvoiced, p.InvoiceStatus)
		assert.Equal(t, "HYP-0042", p.InvoiceNumber.MustGet())
		assert.Equal(t, today, p.InvoicedOn.MustGet().String())
		assert.NotNil(t, p.Invoicing.SentAt.MustGet())
		assert.Equal(t, start.AddDate(0, 0, 45).Format(time.DateOnly), p.Invoicing.DueOn.String())

		msgs := mailServer.To("ken." + tenant.ID + "@majufoods.test")
		require.Len(t, msgs, 1)
		assert.Contains(t, msgs[0].Data, "Subject: Invoice HYP-0042 for Siti Aisyah")
		assert.Contains(t, partsOf(t, msgs[0].Data), "application/pdf Invoice HYP-0042.pdf")

		var s api.InvoiceSettings
		a.request(http.MethodGet, "/v1/invoice-settings", recruiter, nil).JSON(&s)
		assert.Equal(t, 43, s.NextNumber, "the next invoice's number")

		res := a.request(http.MethodPost, "/v1/placements/"+p.Id+"/invoice", recruiter, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "never twice")
		file := a.request(http.MethodGet, "/v1/placements/"+p.Id+"/invoice/file?format=docx", recruiter, nil)
		assert.Contains(t, docxText(t, file.Body), "HYP-0042", "the invoice as sent")
	})

	t.Run("an invoice needs a fee, and waits for the start date", func(t *testing.T) {
		later := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Ahmad", "lastName": "Tarmizi"}).Id)
		q := a.placement(http.MethodPost, "/v1/applications/"+later.Id+"/placement", recruiter, map[string]any{
			"startDate": start.AddDate(0, 1, 0).Format(time.DateOnly)}, http.StatusCreated)
		res := a.request(http.MethodPost, "/v1/placements/"+q.Id+"/invoice", recruiter, nil)
		require.Equal(t, http.StatusUnprocessableEntity, res.Status)
		assert.Equal(t, "Record the placement's fee first.", res.Error().Error.Message)

		q = a.placement(http.MethodPut, "/v1/placements/"+q.Id+"/invoicing", recruiter, map[string]any{
			"contactId": hiring.Id, "paymentTermsDays": 30, "dueOn": nil, "autoSend": true}, http.StatusOK)
		_, err := testutil.Current.Admin.Exec(context.Background(),
			`UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'placement.invoice' AND status = 'pending'`, tenant.ID)
		require.NoError(t, err)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		q = a.placements(recruiter, "?applicationId="+later.Id).Items[0]
		assert.Equal(t, api.InvoiceStatusNotInvoiced, q.InvoiceStatus, "not before the start date")
		var waiting int
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'placement.invoice' AND status = 'pending' AND run_after > now() + interval '20 days'`,
			tenant.ID).Scan(&waiting))
		assert.Equal(t, 1, waiting, "queued again for the start date")
	})
}

