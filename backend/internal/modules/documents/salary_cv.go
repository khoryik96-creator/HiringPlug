package documents

// Formatted CVs made with the candidate's salary: only someone who may see
// salaries (candidates.compensation:read) may open one, download it or its
// PDF, read its text or attach it to an email; for anyone else it is
// FORBIDDEN, as the database already refuses the salary itself. Opening one
// is counted in the log of who was shown a salary (salaryview).

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// SalaryCVs tells formatted CVs made with the salary (cvformats module), in
// the caller's transaction.
type SalaryCVs interface {
	IncludesSalary(ctx context.Context, tx pgx.Tx, documentID string) (bool, error)
}

// CheckSalaryCVsWith keeps formatted CVs made with the salary from those
// who may not see salaries.
func (m *Module) CheckSalaryCVsWith(c SalaryCVs) { m.service.salaryCVs = c }

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.service.salaryViews = r }

// salaryCV reports whether row is a formatted CV made with the salary, and
// refuses it to a transaction whose user may not see salaries.
func (s *Service) salaryCV(ctx context.Context, tx pgx.Tx, row store.Document) (bool, error) {
	salary, err := s.includesSalary(ctx, tx, row.ID, row.Source)
	if err != nil || !salary {
		return false, err
	}
	may, err := store.New(tx).MaySeeSalary(ctx)
	if err != nil {
		return false, err
	}
	if !may {
		return false, apperr.New(api.ErrorCodeFORBIDDEN, "This formatted CV includes the candidate's salary, which your role may not see.")
	}
	return true, nil
}

// includesSalary reports whether a document is a formatted CV made with
// the salary.
func (s *Service) includesSalary(ctx context.Context, tx pgx.Tx, id, source string) (bool, error) {
	if source != "formatted" || s.salaryCVs == nil {
		return false, nil
	}
	return s.salaryCVs.IncludesSalary(ctx, tx, id)
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
