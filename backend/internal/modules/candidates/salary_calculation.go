package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/salary"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// SalaryRules is the company's saved salary calculation choices (settings
// module). The calculator reads them itself: a browser cannot pick a formula.
type SalaryRules interface {
	SalaryCalculationRules(ctx context.Context, tx pgx.Tx) (api.SalaryCalculationRules, error)
}

// CalculateSalary previews values only. Saving remains the explicit, versioned
// screening-call/compensation operation, with the existing salary permissions.
func (h *HTTP) CalculateSalary(ctx context.Context, req api.CalculateSalaryRequestObject) (api.CalculateSalaryResponseObject, error) {
	out, err := h.svc.calculateSalary(ctx, authz.MustFromContext(ctx), *req.Body)
	if err != nil {
		return nil, err
	}
	return api.CalculateSalary200JSONResponse(out), nil
}

func (s *Service) calculateSalary(ctx context.Context, p authz.Principal, in api.SalaryCalculationInput) (api.SalaryCalculation, error) {
	var saved api.SalaryCalculationRules
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		saved, err = s.salaryRules.SalaryCalculationRules(ctx, tx)
		return err
	})
	if err != nil {
		return api.SalaryCalculation{}, err
	}
	rules := salary.Rules{Allowances: saved.Allowances, Bonus: saved.Bonus, BonusOnAllowances: saved.BonusOnAllowances, Cents: saved.Cents}
	r := salary.Calculate(in.Current, in.Expected, rules, deref(in.CurrentCurrency))
	out := api.SalaryCalculation{
		HasCurrent: r.HasCurrent, ReviewRequired: r.ReviewRequired,
		Base: r.Base.String(), Allowances: r.Allowances.String(), Bonus: r.Bonus.String(), Monthly: r.Monthly.String(),
		AnnualMonths: r.AnnualMonths, Warnings: r.Warnings, Excluded: r.Excluded,
	}
	if r.CurrentCurrency != "" {
		out.CurrentCurrency = &r.CurrentCurrency
	}
	if r.ExpectedCurrency != "" {
		out.ExpectedCurrency = &r.ExpectedCurrency
	}
	if r.ExpectedMin != nil {
		n := r.ExpectedMin.String()
		out.ExpectedMin = &n
	}
	if r.ExpectedMax != nil {
		n := r.ExpectedMax.String()
		out.ExpectedMax = &n
	}
	return out, nil
}
