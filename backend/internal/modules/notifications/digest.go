package notifications

// The morning email: one job per user per morning, each queueing the next.
// Turning the email on starts a chain (SetMorningEmail); a job does nothing
// when the email was turned off, or turned off and on again (the newer
// chain carries on), or the user is no longer active. A morning with
// nothing to say sends nothing.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notifications/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func (s *Service) sendDigest(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.NotificationDigestJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	state, err := q.LockState(ctx, payload.UserId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !state.MorningEmail || int(state.DigestChain) != payload.Chain {
		return nil
	}
	p, active, err := s.users.PrincipalFor(ctx, tx, payload.UserId)
	if err != nil || !active {
		return err
	}
	now := s.now()
	today := WindowAt(now, s.cfg.TimeZone).Today
	if !state.EmailedOn.Valid || !state.EmailedOn.Time.Equal(today) {
		var items []Item
		// As the user, so the email holds only what they may see.
		if err := database.AsPrincipal(ctx, tx, p, func() error {
			items, _, err = s.items(ctx, tx, p, state, now)
			return err
		}); err != nil {
			return err
		}
		items = DigestFor(items, now)
		if subject, body, ok := DigestText(items, s.cfg.PublicURL); ok {
			sent, err := s.mail.EmailUser(ctx, tx, job.TenantID, p.UserID, subject, body)
			if err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				Action: "notification.morning_email_sent", EntityType: "user", EntityID: p.UserID,
				Changes: map[string]any{"items": len(items), "emailed": sent},
			}); err != nil {
				return err
			}
		}
		if err := q.MarkEmailed(ctx, store.MarkEmailedParams{UserID: p.UserID, Day: pgtype.Date{Time: today, Valid: true}}); err != nil {
			return err
		}
	}
	_, err = jobqueue.EnqueueAt(ctx, tx, job.TenantID, api.JobKindNotificationsDigest,
		api.NotificationDigestJob{UserId: payload.UserId, Chain: payload.Chain}, NextMorning(now, s.cfg.TimeZone))
	return err
}
