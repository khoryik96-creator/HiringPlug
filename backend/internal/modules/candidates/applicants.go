package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Applicant is someone applying through the careers page.
type Applicant struct {
	FirstName, LastName string
	Email               string
	Phone               *string
}

// FindOrCreateApplicant returns the candidate with the applicant's email, or
// adds them (source: careers page), in the caller's transaction. An existing
// candidate is left unchanged: a public form must not overwrite what the
// team recorded. created says which happened.
func (m *Module) FindOrCreateApplicant(ctx context.Context, tx pgx.Tx, a Applicant) (id string, created bool, err error) {
	source := "careers_page"
	profile, errs := NormalizeProfile(Profile{
		FirstName: a.FirstName, LastName: a.LastName, Email: &a.Email, Phone: a.Phone, Source: &source,
	}, m.service.phoneRegion)
	if err := errs.Err(); err != nil {
		return "", false, err
	}
	q := store.New(tx)
	existing, err := q.FindCandidateIDByEmail(ctx, profile.Email)
	if err == nil {
		return existing, false, nil
	}
	if !database.IsNoRows(err) {
		return "", false, err
	}
	row, err := q.InsertCandidate(ctx, insertParams(profile))
	if err != nil {
		return "", false, err
	}
	err = audit.Record(ctx, tx, audit.Entry{
		Action: "candidate.created", EntityType: "candidate", EntityID: row.ID, Changes: map[string]any{"via": "careers page"},
	})
	return row.ID, true, err
}
