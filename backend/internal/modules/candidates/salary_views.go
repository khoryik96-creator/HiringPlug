package candidates

// Who was shown a salary: a candidate's page with their salary (and the
// candidate a change sends back with it: a profile change, their history, a
// merge, a screening call that did not set it), their screening calls'
// salaries, a list or search page with salaries and an export with them
// are each counted (salaryview), in the transaction that read them. What other modules read through this module's public API (the
// AI, emails, formatted CVs) is counted where they show it, if at all.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.service.salaryViews = r }

// HasSalary: salary details with a current or expected salary (notes
// alone, or none, are not a salary shown). Lists, pages and formatted CVs
// count by the same rule.
func HasSalary(c *Compensation) bool { return c != nil && (c.Current != nil || c.Expected != nil) }

// salariesShown are the candidates of a page whose current or expected
// salary is in it.
func salariesShown(items []Summary) []string {
	var out []string
	for _, s := range items {
		if HasSalary(s.Compensation) {
			out = append(out, s.ID)
		}
	}
	return out
}

// salarySort: a sort by current or expected salary (CandidateSort).
func salarySort(sort string) bool { return strings.Contains(sort, "salary") }

// probedOrShown are the candidates of a page whose salary it tells about:
// every one on it when salaries chose or ordered them (probed: the expected
// salary filter, a salary sort; even without salaries in the page, who is
// found and in which order says something about each), and those whose
// salary is in it.
func probedOrShown(items []Summary, probed bool) []string {
	out := salariesShown(items)
	if probed {
		for _, s := range items {
			out = append(out, s.ID)
		}
	}
	return out
}

// pageShown counts a candidate sent back with their salary (a page, or the
// candidate as a change left them), in tx.
func (s *Service) pageShown(ctx context.Context, tx pgx.Tx, view View) error {
	if !HasSalary(view.Compensation) {
		return nil
	}
	return salaryview.In(ctx, tx, s.salaryViews, salaryview.Candidate, view.ID)
}
