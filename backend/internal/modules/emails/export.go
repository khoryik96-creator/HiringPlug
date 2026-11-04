package emails

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
)

// Exported is an email as it goes into a copy of a candidate's data.
type Exported struct {
	To                    []string
	Subject, Body, Status string
	CreatedAt             time.Time
	SentAt                *time.Time
}

// ForExport returns the emails about a candidate, newest first, in the
// caller's transaction.
func (m *Module) ForExport(ctx context.Context, tx pgx.Tx, candidateID string) ([]Exported, error) {
	rows, err := store.New(tx).ListCandidateEmails(ctx, &candidateID)
	if err != nil {
		return nil, err
	}
	out := make([]Exported, len(rows))
	for i, r := range rows {
		out[i] = Exported{To: r.ToAddresses, Subject: r.Subject, Body: r.Body, Status: r.Status, CreatedAt: r.CreatedAt, SentAt: r.SentAt}
	}
	return out, nil
}
