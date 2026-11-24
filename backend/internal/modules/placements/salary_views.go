package placements

// Who was shown a salary (salaryview): a placement's agreed salary is
// counted where the app shows it - an application's placement - and in an
// export with fees. The dashboard's list does not show it and is not
// counted; nor is an invoice (its fee basis may name the salary).

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
