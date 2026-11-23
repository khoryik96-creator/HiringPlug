package notifications

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notifications/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Service holds the notifications use cases.
type Service struct {
	pool  *pgxpool.Pool
	src   Sources
	users Users
	mail  Mailer
	cfg   Settings
	now   func() time.Time
}

// List is a user's notifications.
type List struct {
	Items  []Item
	Unseen int
	AsOf   time.Time // when it was worked out (marking seen goes up to here)
	Prefs
}

// Prefs is what a user chose: the morning email, and the kinds turned off.
type Prefs struct {
	MorningEmail bool
	Muted        []string
}

// List returns the user's notifications, newest first.
func (s *Service) List(ctx context.Context, p authz.Principal) (List, error) {
	var out List
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		state, err := s.state(ctx, tx, p.UserID)
		if err != nil {
			return err
		}
		now := s.now()
		all, err := s.items(ctx, tx, p, state, now)
		items, unseen := Shown(all)
		out = List{Items: items, Unseen: unseen, AsOf: now, Prefs: Prefs{MorningEmail: state.MorningEmail, Muted: state.MutedKinds}}
		return err
	})
	return out, err
}

// MarkSeen marks as seen what the user was shown: everything up to upTo
// (the list's AsOf; never later than now), so what came after stays new.
func (s *Service) MarkSeen(ctx context.Context, p authz.Principal, upTo time.Time) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).MarkSeen(ctx, store.MarkSeenParams{UserID: p.UserID, UpTo: upTo}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "notification.seen", EntityType: "user", EntityID: p.UserID, Changes: map[string]any{"upTo": upTo},
		})
	})
}

// Stalled is how long past its time a morning email's job may be waiting
// (retrying) before saving the settings starts a new chain.
const Stalled = 2 * time.Hour

// PutPrefs saves what the user chose. Turning the morning email on queues
// the next morning's email (a new chain: any job still waiting from before
// does nothing); so does saving while it is on but its chain has stopped
// (its job failed for good).
func (s *Service) PutPrefs(ctx context.Context, p authz.Principal, in Prefs) (Prefs, error) {
	muted, errs := NormalizeMuted(in.Muted)
	if err := errs.Err(); err != nil {
		return Prefs{}, err
	}
	var out Prefs
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		// Locked, so settings saved twice at once start one chain, not two.
		before, err := q.LockState(ctx, p.UserID)
		if database.IsNoRows(err) {
			before, err = store.NotificationState{UserID: p.UserID, MutedKinds: []string{}}, nil
		}
		if err != nil {
			return err
		}
		now := s.now()
		stalled := before.MorningEmail && (before.DigestDueAt == nil || before.DigestDueAt.Before(now.Add(-Stalled)))
		restart := in.MorningEmail && (!before.MorningEmail || stalled)
		due := NextMorning(now, s.cfg.TimeZone)
		row, err := q.PutSettings(ctx, store.PutSettingsParams{UserID: p.UserID, MorningEmail: in.MorningEmail, MutedKinds: muted, Restart: restart, DueAt: &due})
		if err != nil {
			return err
		}
		out = Prefs{MorningEmail: row.MorningEmail, Muted: row.MutedKinds}
		if !restart && before.MorningEmail == in.MorningEmail && slices.Equal(NormalizeMutedSilently(before.MutedKinds), muted) {
			return nil
		}
		if restart {
			if _, err := jobqueue.EnqueueAt(ctx, tx, p.TenantID, api.JobKindNotificationsDigest,
				api.NotificationDigestJob{UserId: p.UserID, Chain: int(row.DigestChain)}, due); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "notification.settings_changed", EntityType: "user", EntityID: p.UserID,
			Changes: map[string]any{"morningEmail": in.MorningEmail, "mutedKinds": muted},
		})
	})
	return out, err
}

// state returns the user's row, or the defaults when there is none.
func (s *Service) state(ctx context.Context, tx pgx.Tx, userID string) (store.NotificationState, error) {
	row, err := store.New(tx).GetState(ctx, userID)
	if database.IsNoRows(err) {
		return store.NotificationState{UserID: userID, MutedKinds: []string{}}, nil
	}
	return row, err
}

// items gathers the records the user may see (in tx, scoped to them) and
// makes all their notifications, leaving out the kinds they turned off.
func (s *Service) items(ctx context.Context, tx pgx.Tx, p authz.Principal, state store.NotificationState, now time.Time) ([]Item, error) {
	w := WindowAt(now, s.cfg.TimeZone)
	wants := func(kind string) bool { return !slices.Contains(state.MutedKinds, kind) }
	var in Inputs
	var err error
	if p.Can(api.PermissionPipelineRead) && wants(KindApplicant) {
		jobs, err := s.src.OwnedJobs(ctx, tx, p.UserID)
		if err != nil {
			return nil, err
		}
		if in.Applicants, err = s.src.Applicants(ctx, tx, jobs, w.ApplicantsSince); err != nil {
			return nil, err
		}
	}
	if p.Can(api.PermissionAutomationsTasksRead) && wants(KindTask) {
		if in.Tasks, err = s.src.DueTasks(ctx, tx, p.UserID, w.Today); err != nil {
			return nil, err
		}
	}
	if p.Can(api.PermissionOffersApprove) && wants(KindOfferApproval) {
		if in.Offers, err = s.src.OffersToApprove(ctx, tx, p.UserID); err != nil {
			return nil, err
		}
	}
	if p.Can(api.PermissionInterviewsRead) && wants(KindInterview) {
		if in.Interviews, err = s.src.Interviews(ctx, tx, p.UserID, w.From, w.To); err != nil {
			return nil, err
		}
	}
	if p.Can(api.PermissionCvformatsCreate) && wants(KindCVFormatted) {
		if in.FormattedCVs, err = s.src.FormattedCVs(ctx, tx, p.UserID, w.FormattedSince); err != nil {
			return nil, err
		}
	}
	var ids []string
	for _, r := range in.Applicants {
		ids = append(ids, r.ApplicationID)
	}
	for _, r := range in.Offers {
		ids = append(ids, r.ApplicationID)
	}
	for _, r := range in.Interviews {
		ids = append(ids, r.ApplicationID)
	}
	for _, t := range in.Tasks {
		if t.ApplicationID != nil {
			ids = append(ids, *t.ApplicationID)
		}
	}
	if len(ids) > 0 {
		if in.Apps, err = s.src.Applications(ctx, tx, ids); err != nil {
			return nil, err
		}
	}
	return Build(in, state.SeenAt, w), nil
}
