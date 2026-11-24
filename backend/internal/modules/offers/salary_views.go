package offers

// Who was shown a salary (salaryview): an offer's salary is counted where
// it is shown - a list that asked for salaries (an application's offers, the
// offers waiting for approval). Lists that do not show it (offers sent, on
// the dashboard and the pipeline; checking whether an offer is open) do not
// ask, and get none.

import (
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.HTTP.svc.salaryViews = r }

// withoutSalaries leaves the salaries out of views (a list that did not
// ask for them).
func withoutSalaries(views []View) {
	for i := range views {
		views[i].Salary = nil
	}
}

// offeredTo are the candidates whose offered salary is among views.
func offeredTo(views []View) []string {
	var out []string
	for _, v := range views {
		if v.Salary != nil {
			out = append(out, v.App.CandidateID)
		}
	}
	return out
}
