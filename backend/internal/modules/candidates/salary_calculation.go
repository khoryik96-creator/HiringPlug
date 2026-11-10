package candidates

import (
	"context"
	"strconv"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salarycalc"
)

// CalculateSalary previews values only. Saving remains the explicit, versioned
// screening-call/compensation operation, with the existing salary permissions.
func (h *HTTP) CalculateSalary(_ context.Context, req api.CalculateSalaryRequestObject) (api.CalculateSalaryResponseObject, error) {
	b := req.Body
	r := salarycalc.Calculate(b.Current, b.Expected, salarycalc.Rules{Allowances: b.Rules.Allowances, Bonus: b.Rules.Bonus, BonusOnAllowances: b.Rules.BonusOnAllowances, Cents: b.Rules.Cents})
	decimal := func(n float64) string { return strconv.FormatFloat(n, 'f', 2, 64) }
	out := api.SalaryCalculation{Base: decimal(r.Base), Allowances: decimal(r.Allowances), Bonus: decimal(r.Bonus), Monthly: decimal(r.Monthly), AnnualMonths: r.AnnualMonths, Warnings: r.Warnings, Excluded: r.Excluded}
	if currency := salarycalc.Currency(b.Current); currency != "" {
		out.CurrentCurrency = &currency
	}
	if currency := salarycalc.Currency(b.Expected); currency != "" {
		out.ExpectedCurrency = &currency
	} else if strings.Contains(b.Expected, "%") || strings.Contains(strings.ToLower(b.Expected), "lateral") || strings.Contains(strings.ToLower(b.Expected), "same") {
		out.ExpectedCurrency = out.CurrentCurrency
	}
	if r.ExpectedMin != nil {
		n := decimal(*r.ExpectedMin)
		out.ExpectedMin = &n
	}
	if r.ExpectedMax != nil {
		n := decimal(*r.ExpectedMax)
		out.ExpectedMax = &n
	}
	return api.CalculateSalary200JSONResponse(out), nil
}
