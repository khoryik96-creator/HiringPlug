package claims

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Ref is a claim as the person loaded it.
type Ref struct {
	ID      string
	Version int
}

// locked locks the claims referred to (in id order, so two requests never
// wait on each other), all of them or none: one missing (or not visible)
// or changed since it was loaded fails the request.
func locked(ctx context.Context, q *store.Queries, refs []Ref) ([]store.Claim, error) {
	ids := make([]string, 0, len(refs))
	version := map[string]int{}
	for _, r := range refs {
		if _, dup := version[r.ID]; dup {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: "claims", Message: "names claim " + r.ID + " twice"}})
		}
		version[r.ID] = r.Version
		ids = append(ids, r.ID)
	}
	rows, err := q.LockClaims(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, apperr.NotFound("claim")
	}
	for _, r := range rows {
		if int(r.Version) != version[r.ID] {
			return nil, apperr.StaleVersion("claim")
		}
	}
	return rows, nil
}

// Submit sends the person's draft or rejected claims for approval: each
// complete, all of them or none.
func (s *Service) Submit(ctx context.Context, p authz.Principal, refs []Ref) ([]api.Claim, error) {
	var out []api.Claim
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := locked(ctx, q, refs)
		if err != nil {
			return err
		}
		var bad apperr.FieldErrors
		for i, r := range rows {
			if r.UserID != p.UserID {
				return apperr.New(api.ErrorCodeFORBIDDEN, "You can submit only your own claims.")
			}
			if !Editable(r.Status) {
				return apperr.New(api.ErrorCodeCLAIMWRONGSTATUS, r.FileName+" is "+r.Status+": only draft and rejected claims are submitted.")
			}
			v, err := valuesOf(r)
			if err != nil {
				return err
			}
			if missing := Missing(v); len(missing) > 0 {
				words := make([]string, 0, len(missing))
				for _, f := range missing {
					words = append(words, Labels[f])
				}
				bad.Add(fmt.Sprintf("claims[%d]", i), r.FileName+": fill in the "+strings.Join(words, ", "))
			}
		}
		if err := bad.Err(); err != nil {
			return err
		}
		saved := make([]store.Claim, 0, len(rows))
		for _, r := range rows {
			row, err := q.SubmitClaim(ctx, store.SubmitClaimParams{ID: r.ID, Version: r.Version})
			if err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{Action: "claim.submitted", EntityType: "claim", EntityID: r.ID}); err != nil {
				return err
			}
			saved = append(saved, row)
		}
		if s.storage != nil {
			ids := make([]string, 0, len(saved))
			for _, r := range saved {
				ids = append(ids, r.ID)
			}
			if err := s.storage.Submitted(ctx, tx, p.TenantID, ids); err != nil {
				return err
			}
			if saved, err = q.LockClaims(ctx, ids); err != nil { // with their upload queued
				return err
			}
		}
		out, err = s.views(ctx, tx, p, saved)
		return err
	})
	return out, err
}

// Withdraw takes the person's submitted claim back to draft, before it is
// decided.
func (s *Service) Withdraw(ctx context.Context, p authz.Principal, id string, version int) (api.Claim, error) {
	var out api.Claim
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := lockOwn(ctx, q, p, id)
		if err != nil {
			return err
		}
		if int(row.Version) != version {
			return apperr.StaleVersion("claim")
		}
		if row.Status != Submitted {
			return apperr.New(api.ErrorCodeCLAIMWRONGSTATUS, "This claim is "+row.Status+": only a submitted claim not yet decided can be withdrawn.")
		}
		saved, err := q.WithdrawClaim(ctx, store.WithdrawClaimParams{ID: id, Version: row.Version})
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "claim.withdrawn", EntityType: "claim", EntityID: id}); err != nil {
			return err
		}
		views, err := s.views(ctx, tx, p, []store.Claim{saved})
		if err == nil {
			out = views[0]
		}
		return err
	})
	return out, err
}

// Review approves or rejects submitted claims, or marks approved ones paid
// (approvers): all of them or none.
func (s *Service) Review(ctx context.Context, p authz.Principal, refs []Ref, decision string, note *string) ([]api.Claim, error) {
	note, ok := CleanText(note, MaxNote)
	switch {
	case !ok:
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "note", Message: "is too long"}})
	case decision == Reject && note == nil:
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "note", Message: "say why the claims are rejected"}})
	case decision != Reject:
		note = nil // only a rejection carries a note
	}
	var out []api.Claim
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := locked(ctx, q, refs)
		if err != nil {
			return err
		}
		saved := make([]store.Claim, 0, len(rows))
		for _, r := range rows {
			to, ok := Decide(decision, r.Status)
			if !ok {
				return apperr.New(api.ErrorCodeCLAIMWRONGSTATUS, r.FileName+" is "+r.Status+": submitted claims are approved or rejected, approved ones paid.")
			}
			var row store.Claim
			if to == Paid {
				row, err = q.PayClaim(ctx, store.PayClaimParams{ID: r.ID, Version: r.Version})
			} else {
				row, err = q.DecideClaim(ctx, store.DecideClaimParams{ID: r.ID, Version: r.Version, Status: to, ReviewNote: note})
			}
			if err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{Action: "claim." + to, EntityType: "claim", EntityID: r.ID}); err != nil {
				return err
			}
			saved = append(saved, row)
		}
		out, err = s.views(ctx, tx, p, saved)
		return err
	})
	return out, err
}
