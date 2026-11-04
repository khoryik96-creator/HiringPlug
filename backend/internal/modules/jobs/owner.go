package jobs

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// ownerFor is who a job added by p belongs to: p, if p is a person (not an
// API key).
func ownerFor(p authz.Principal) *string {
	if !strings.HasPrefix(p.UserID, "usr_") {
		return nil
	}
	id := p.UserID
	return &id
}

// SetOwner gives a job to another active user of the company, or to nobody.
// Roles that see the candidates on their own jobs see this job's candidates
// through its owner.
func (s *Service) SetOwner(ctx context.Context, p authz.Principal, id string, owner *string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getRow(ctx, q, id)
		if err != nil {
			return err
		}
		if owner != nil {
			active, err := s.users.ActiveUsers(ctx, tx, []string{*owner})
			if err != nil {
				return err
			}
			if _, ok := active[*owner]; !ok {
				var errs apperr.FieldErrors
				errs.Add("ownerId", "is not an active user of your company")
				return errs.Err()
			}
		}
		if _, err := q.SetJobOwner(ctx, store.SetJobOwnerParams{ID: id, OwnerID: owner}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "job.owner_changed", EntityType: "job", EntityID: id,
			Changes: map[string]any{"ownerId": map[string]any{"from": row.OwnerID, "to": owner}},
		})
	})
}
