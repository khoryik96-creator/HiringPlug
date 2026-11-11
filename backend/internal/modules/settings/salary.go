package settings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Salary rules contain no candidate values; salary input never enters settings.
func defaultSalaryRules() api.SalaryCalculationRules {
	return api.SalaryCalculationRules{Allowances: true, Bonus: true}
}

func (s *Service) salaryRules(ctx context.Context, p authz.Principal) (api.SalaryCalculationRules, error) {
	out := defaultSalaryRules()
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		v, err := store.New(tx).GetPolicy(ctx, "salary_calculation")
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(v), &out)
	})
	return out, err
}

func (s *Service) setSalaryRules(ctx context.Context, p authz.Principal, rules api.SalaryCalculationRules) error {
	value, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).SetPolicy(ctx, store.SetPolicyParams{Key: "salary_calculation", Value: string(value)}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "settings.salary_calculation_changed", EntityType: "settings", EntityID: p.TenantID, Changes: map[string]any{"rules": rules}})
	})
	return err
}

func (h *HTTP) GetSalaryCalculationRules(ctx context.Context, _ api.GetSalaryCalculationRulesRequestObject) (api.GetSalaryCalculationRulesResponseObject, error) {
	rules, err := h.svc.salaryRules(ctx, authz.MustFromContext(ctx))
	return api.GetSalaryCalculationRules200JSONResponse(rules), err
}

func (h *HTTP) GetScreeningSalaryCalculationRules(ctx context.Context, _ api.GetScreeningSalaryCalculationRulesRequestObject) (api.GetScreeningSalaryCalculationRulesResponseObject, error) {
	rules, err := h.svc.salaryRules(ctx, authz.MustFromContext(ctx))
	return api.GetScreeningSalaryCalculationRules200JSONResponse(rules), err
}

func (h *HTTP) PutSalaryCalculationRules(ctx context.Context, req api.PutSalaryCalculationRulesRequestObject) (api.PutSalaryCalculationRulesResponseObject, error) {
	err := h.svc.setSalaryRules(ctx, authz.MustFromContext(ctx), *req.Body)
	return api.PutSalaryCalculationRules200JSONResponse(*req.Body), err
}

// SalaryEmailBreakdown reads the email-only presentation preference in the caller's transaction.
func (m *Module) SalaryEmailBreakdown(ctx context.Context, tx pgx.Tx) (bool, error) {
	value, err := store.New(tx).GetPolicy(ctx, "salary_calculation")
	if database.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var rules struct {
		EmailSalaryDisplay string `json:"emailSalaryDisplay"`
	}
	err = json.Unmarshal([]byte(value), &rules)
	return rules.EmailSalaryDisplay == "breakdown", err
}
