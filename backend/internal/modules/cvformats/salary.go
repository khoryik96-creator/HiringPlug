package cvformats

// Formatted CVs made with the salary: only people who may see salaries may
// open them (the documents module asks IncludesSalary), and making one is
// counted in the log of who was shown a salary.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// IncludesSalary reports whether a document is a formatted CV HyPlug
// made with the candidate's salary, in the caller's transaction (false for
// any other document, or one the user may not see).
func (m *Module) IncludesSalary(ctx context.Context, tx pgx.Tx, documentID string) (bool, error) {
	made, err := store.New(tx).GetFormattedCv(ctx, documentID)
	if database.IsNoRows(err) {
		return false, nil
	}
	return made.IncludeSalary, err
}

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.svc.salaryViews = r }
