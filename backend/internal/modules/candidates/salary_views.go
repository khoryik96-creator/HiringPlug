package candidates

// Who was shown a salary: a candidate's page with their salary, their
// screening calls' salaries, a list or search page with salaries and an
// export with them are each counted (salaryview), in the transaction that
// read them. What other modules read through this module's public API (the
// AI, emails, formatted CVs) is counted where they show it, if at all.

import (
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.service.salaryViews = r }

// salariesShown are the candidates of a page whose current or expected
// salary is in it.
func salariesShown(items []Summary) []string {
	var out []string
	for _, s := range items {
		if c := s.Compensation; c != nil && (c.Current != nil || c.Expected != nil) {
			out = append(out, s.ID)
		}
	}
	return out
}
