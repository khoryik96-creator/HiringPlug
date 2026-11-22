package pipeline

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
)

// Applicant is an application someone made themselves (a careers page, a job
// board, a referral), and when.
type Applicant struct {
	ID string
	At time.Time
}

// ApplicantsSince returns who applied to these jobs after since, newest
// first (at most 50), in the caller's transaction: applications a recruiter
// added are left out.
func (m *Module) ApplicantsSince(ctx context.Context, tx pgx.Tx, jobIDs []string, since time.Time) ([]Applicant, error) {
	if len(jobIDs) == 0 {
		return nil, nil
	}
	rows, err := store.New(tx).ApplicantsSince(ctx, store.ApplicantsSinceParams{JobIds: jobIDs, Since: since})
	if err != nil {
		return nil, err
	}
	out := make([]Applicant, len(rows))
	for i, r := range rows {
		out[i] = Applicant{ID: r.ID, At: r.CreatedAt}
	}
	return out, nil
}
