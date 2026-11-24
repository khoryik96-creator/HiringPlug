package placements

// Who was shown a salary (salaryview): a placement's agreed salary is
// counted where it is shown - a list that asked for it (an application's
// placement, a placement being edited on the dashboard) - and in an export
// with fees. The dashboard's list does not ask, and gets none; an invoice
// is not counted (its fee basis may name the salary).

import (
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
