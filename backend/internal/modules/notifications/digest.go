package notifications

// The morning email: one job per user per morning, each queueing the next.
// Turning the email on starts a chain (Service.PutPrefs); a job does nothing
// and ends the chain when the email was turned off, or turned off and on
// again (the newer chain carries on). While the user is inactive or may not
// get notifications, mornings pass without an email but the chain goes on,
// so the email comes back with them. A morning with nothing to say sends
// nothing. Every morning's run is audited.

import (
	"context"
	"time"

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
	if err != nil {
		return err
	}
	now := s.now()
	today := WindowAt(now, s.cfg.TimeZone).Today
	ran := map[string]any{"items": 0, "emailed": false}
	var emailed *time.Time
	switch {
	case !active || !p.Can(api.PermissionNotificationsRead):
		ran["skipped"] = "the user is inactive or may not get notifications"
	case state.EmailedOn.Valid && state.EmailedOn.Time.Equal(today):
		ran["skipped"] = "already emailed today"
	default:
		var items []Item
		// As the user, so the email holds only what they may see.
		if err := database.AsPrincipal(ctx, tx, p, func() error {
			items, err = s.items(ctx, tx, p, state, now)
			return err
		}); err != nil {
			return err
		}
		var last time.Time
		if state.EmailedOn.Valid {
			last = state.EmailedOn.Time
		}
		items = DigestFor(items, DigestSince(last, now, s.cfg.TimeZone))
		if subject, body, ok := DigestText(items, s.cfg.PublicURL); ok {
			sent, err := s.mail.EmailUser(ctx, tx, job.TenantID, p.UserID, subject, body)
			if err != nil {
				return err
			}
			ran["items"], ran["emailed"] = len(items), sent
			emailed = &today
		}
	}
	next := NextDayMorning(now, s.cfg.TimeZone)
	if err := q.DigestRan(ctx, store.DigestRanParams{UserID: payload.UserId, DueAt: &next, Emailed: dateOrNull(emailed)}); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: "notification.morning_email_ran", EntityType: "user", EntityID: payload.UserId, Changes: ran,
	}); err != nil {
		return err
	}
	_, err = jobqueue.EnqueueAt(ctx, tx, job.TenantID, api.JobKindNotificationsDigest,
		api.NotificationDigestJob{UserId: payload.UserId, Chain: payload.Chain}, next)
	return err
}

func dateOrNull(day *time.Time) pgtype.Date {
	if day == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *day, Valid: true}
}
