package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func rate(from, below any, feePercent string, paymentTermsDays, guaranteeDays int, remarks any) map[string]any {
	return map[string]any{"salaryFrom": from, "salaryBelow": below, "feePercent": feePercent, "paymentTermsDays": paymentTermsDays,
		"guaranteeDays": guaranteeDays, "remarks": remarks}
}

func TestTieredTermsOfBusinessFillPlacementsBySalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Maju Foods Berhad"})
	terms := "/v1/companies/" + company.Id + "/terms"
	put := func(version int, tiers ...any) *response {
		return a.request(http.MethodPut, terms, recruiter, map[string]any{"version": version, "tiers": tiers, "salaryCurrency": "MYR", "signedOn": nil, "notes": nil})
	}

	t.Run("a salary falls in one rate only", func(t *testing.T) {
		res := put(0, rate(nil, "120000", "18", 14, 60, nil), rate("100000", nil, "25", 30, 90, nil))
		assert.Equal(t, []string{"tiers.1.salaryFrom"}, fieldNames(t, res.Error()))
		res = put(0, rate("120000", "100000", "18", 14, 60, nil))
		assert.Equal(t, []string{"tiers.0.salaryBelow"}, fieldNames(t, res.Error()))
	})

	res := put(0,
		rate(nil, "120000", "18", 14, 60, nil),
		rate("120000", nil, "25", 30, 90, " 50% on signing, 50% on the start date "),
		map[string]any{"salaryFrom": nil, "salaryBelow": nil, "feePercent": nil, "paymentTermsDays": nil, "guaranteeDays": nil, "remarks": "  "}, // an empty row is dropped
	)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var got api.CompanyTerms
	res.JSON(&got)
	require.Len(t, got.Tiers, 2)
	assert.Equal(t, "MYR", got.SalaryCurrency)
	assert.Equal(t, "120000", got.Tiers[1].SalaryFrom.MustGet())
	assert.Equal(t, "50% on signing, 50% on the start date", got.Tiers[1].Remarks.MustGet())

	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Finance Manager"})
	place := func(name, monthly string) api.Placement {
		appl := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": name}).Id)
		var d api.PlacementDefaults
		a.request(http.MethodGet, "/v1/applications/"+appl.Id+"/placement-defaults", recruiter, nil).JSON(&d)
		assert.Len(t, d.Tiers, 2, "the form picks the rate for the salary typed in")
		assert.True(t, d.FeePercent.IsNull(), "the fee depends on the salary")
		return a.placement(http.MethodPost, "/v1/applications/"+appl.Id+"/placement", recruiter, map[string]any{
			"startDate": time.Now().Format(time.DateOnly), "guaranteeDays": 60,
			"money": map[string]any{"salary": map[string]any{"amount": monthly, "currency": "MYR", "period": "month"}, "feePercent": "18"},
		}, http.StatusCreated)
	}
	assert.Equal(t, 14, place("Siti", "8000").Invoicing.PaymentTermsDays, "RM 96,000 a year: below 120,000")
	assert.Equal(t, 30, place("Aina", "10000").Invoicing.PaymentTermsDays, "RM 120,000 a year: from 120,000")
}
