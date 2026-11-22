package jobs

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// SalaryBudget is a job's salary budget, for other modules (reports: the fee
// a placement would bring). Max is nil when only one figure is set.
type SalaryBudget struct {
	Min              string
	Max              *string
	Currency, Period string // period: hour, day, month, year
}

// SalaryBudgets returns the budgets of those of the given jobs that have one,
// in the caller's transaction (none without jobs.salary:read: the database
// leaves them out).
func (m *Module) SalaryBudgets(ctx context.Context, tx pgx.Tx, ids []string) (map[string]SalaryBudget, error) {
	rows, err := store.New(tx).JobSalaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]SalaryBudget, len(rows))
	for _, r := range rows {
		minimum, err := database.DecimalString(r.MinAmount)
		if err != nil {
			return nil, err
		}
		if minimum == nil { // never: the column is required
			continue
		}
		maximum, err := database.DecimalString(r.MaxAmount)
		if err != nil {
			return nil, err
		}
		out[r.JobOrderID] = SalaryBudget{Min: *minimum, Max: maximum, Currency: r.Currency, Period: r.Period}
	}
	return out, nil
}
