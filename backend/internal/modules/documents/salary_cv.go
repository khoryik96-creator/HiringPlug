package documents

// Formatted CVs made with the candidate's salary: opening one is counted in
// the log of who was shown a salary (salaryview).

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// SalaryCVs tells formatted CVs made with the salary (cvformats module), in
// the caller's transaction.
type SalaryCVs interface {
	IncludesSalary(ctx context.Context, tx pgx.Tx, documentID string) (bool, error)
}

// CheckSalaryCVsWith tells formatted CVs made with the salary.
func (m *Module) CheckSalaryCVsWith(c SalaryCVs) { m.service.salaryCVs = c }

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.service.salaryViews = r }

// salaryCV reports whether row is a formatted CV made with the salary.
func (s *Service) salaryCV(ctx context.Context, tx pgx.Tx, row store.Document) (bool, error) {
	if row.Source != "formatted" || s.salaryCVs == nil {
		return false, nil
	}
	return s.salaryCVs.IncludesSalary(ctx, tx, row.ID)
}

// openCV is salaryCV for a CV whose content goes to the user: a salary CV
// is counted as shown.
func (s *Service) openCV(ctx context.Context, tx pgx.Tx, row store.Document) error {
	salary, err := s.salaryCV(ctx, tx, row)
	if err != nil || !salary {
		return err
	}
	return salaryview.In(ctx, tx, s.salaryViews, salaryview.CV, row.CandidateID)
}
