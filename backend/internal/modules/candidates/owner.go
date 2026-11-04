package candidates

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// ownerFor is who a candidate added by p belongs to: p, if p is a person
// (not an API key or the system).
func ownerFor(p authz.Principal) *string {
	if !strings.HasPrefix(p.UserID, "usr_") {
		return nil
	}
	id := p.UserID
	return &id
}

// ownerName is the owner's name (nil without an owner, or for a user who
// is gone).
func ownerName(ctx context.Context, tx pgx.Tx, users Users, owner *string) (*string, error) {
	var name *string
	if owner != nil {
		names, err := users.UserNames(ctx, tx, []string{*owner})
		if err != nil {
			return nil, err
		}
		if n, ok := names[*owner]; ok {
			name = &n
		}
	}
	return name, nil
}

// SetOwner gives a candidate to another user of the company (an active one),
// or to nobody. Who hands a candidate over may then no longer see them, so
// nothing is returned.
func (s *Service) SetOwner(ctx context.Context, p authz.Principal, id string, owner *string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetCandidate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("candidate")
		}
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
		changed, err := q.SetCandidateOwner(ctx, store.SetCandidateOwnerParams{ID: id, OwnerID: owner})
		if err != nil {
			return err
		}
		if !changed {
			return apperr.NotFound("candidate")
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "candidate.owner_changed", EntityType: "candidate", EntityID: id,
			Changes: map[string]any{"ownerId": map[string]any{"from": row.OwnerID, "to": owner}},
		})
	})
}
