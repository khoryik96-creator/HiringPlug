package offers

// Who was shown a salary (salaryview): an offer's salary is counted where
// the app shows it - an application's offers, and the offers waiting for
// approval. Lists that do not show it (offers sent, on the dashboard and
// the pipeline) are not counted.

import (
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.HTTP.svc.salaryViews = r }

// showsSalary: the list asked for is one the app shows offers' salaries in.
func showsSalary(appID, status *string) bool {
	return appID != nil || (status != nil && *status == "pending_approval")
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
