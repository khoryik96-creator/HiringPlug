package candidates

import (
	"context"
	"strconv"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salarycalc"
)

// CalculateSalary previews values only. Saving remains the explicit, versioned
// screening-call/compensation operation, with the existing salary permissions.
func (h *HTTP) CalculateSalary(_ context.Context, req api.CalculateSalaryRequestObject) (api.CalculateSalaryResponseObject, error) {
	b := req.Body
	r := salarycalc.Calculate(b.Current, b.Expected, salarycalc.Rules{Allowances: b.Rules.Allowances, Bonus: b.Rules.Bonus, BonusOnAllowances: b.Rules.BonusOnAllowances, Cents: b.Rules.Cents}, deref(b.CurrentCurrency))
	decimal := func(n float64) string { return strconv.FormatFloat(n, 'f', 2, 64) }
	out := api.SalaryCalculation{HasCurrent: r.HasCurrent, ReviewRequired: r.ReviewRequired, Base: decimal(r.Base), Allowances: decimal(r.Allowances), Bonus: decimal(r.Bonus), Monthly: decimal(r.Monthly), AnnualMonths: r.AnnualMonths, Warnings: r.Warnings, Excluded: r.Excluded}
	if r.CurrentCurrency != "" {
		out.CurrentCurrency = &r.CurrentCurrency
	}
	if r.ExpectedCurrency != "" {
		out.ExpectedCurrency = &r.ExpectedCurrency
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
