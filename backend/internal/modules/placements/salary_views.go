package placements

// Who was shown a salary (salaryview): a placement's agreed salary is
// counted where it is shown - a list that asked for it (an application's
// placement, a placement being edited on the dashboard), an invoice that
// prints it (its fee basis: a share of the annual salary) opened or sent by
// a person - and in an export with fees. The dashboard's list does not ask,
// and gets none.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.service.salaryViews = r }

// salaryOf are the candidates whose agreed salary is among views.
func salaryOf(views []View) []string {
	var out []string
	for _, v := range views {
		if v.Money != nil && v.Money.Salary != nil {
			out = append(out, v.App.CandidateID)
		}
	}
	return out
}

// withoutSalaries leaves the agreed salaries out of views (a list that did
// not ask for them); the fee stays.
func withoutSalaries(views []View) {
	for i, v := range views {
		if v.Money != nil {
			m := *v.Money
			m.Salary = nil
			views[i].Money = &m
		}
	}
}

// sentInvoiceOpened counts the copy of a sent invoice opened, when its fee
// was a share of the salary (the copy is as it was sent: whether its
// template printed the salary is not known, so it is counted).
func (s *Service) sentInvoiceOpened(ctx context.Context, tx pgx.Tx, row store.Placement) error {
	money, err := store.New(tx).PlacementMoney(ctx, []string{row.ID})
	if err != nil || len(money) == 0 {
		return err
	}
	m, err := moneyOf(money[0])
	if err != nil || m == nil || m.Salary == nil || m.FeePercent == nil {
		return err
	}
	apps, err := s.applications.Briefs(ctx, tx, []string{row.ApplicationID})
	if err != nil {
		return err
	}
	return salaryview.In(ctx, tx, s.salaryViews, salaryview.Placement, apps[row.ApplicationID].CandidateID)
}
