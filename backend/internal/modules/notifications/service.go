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
		items, unseen, err := s.items(ctx, tx, p, state, s.now())
		out = List{Items: items, Unseen: unseen, Prefs: Prefs{MorningEmail: state.MorningEmail, Muted: state.MutedKinds}}
		return err
	})
	return out, err
}

// MarkSeen marks everything up to now as seen. Not audited: it records
// that the user looked, not a change to the company's data.
func (s *Service) MarkSeen(ctx context.Context, p authz.Principal) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		return store.New(tx).MarkSeen(ctx, p.UserID)
	})
}

// PutPrefs saves what the user chose. Turning the morning email on queues
// the next morning's email (a new chain: any job still waiting from before
// does nothing).
func (s *Service) PutPrefs(ctx context.Context, p authz.Principal, in Prefs) (Prefs, error) {
	muted, errs := NormalizeMuted(in.Muted)
	if err := errs.Err(); err != nil {
		return Prefs{}, err
	}
	var out Prefs
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		before, err := s.state(ctx, tx, p.UserID)
		if err != nil {
			return err
		}
		row, err := q.PutSettings(ctx, store.PutSettingsParams{UserID: p.UserID, MorningEmail: in.MorningEmail, MutedKinds: muted})
		if err != nil {
			return err
		}
		out = Prefs{MorningEmail: row.MorningEmail, Muted: row.MutedKinds}
		if before.MorningEmail == in.MorningEmail && slices.Equal(NormalizeMutedSilently(before.MutedKinds), muted) {
			return nil
		}
		if in.MorningEmail && !before.MorningEmail {
			if _, err := jobqueue.EnqueueAt(ctx, tx, p.TenantID, api.JobKindNotificationsDigest,
				api.NotificationDigestJob{UserId: p.UserID, Chain: int(row.DigestChain)}, NextMorning(s.now(), s.cfg.TimeZone)); err != nil {
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
// makes their notifications, leaving out the kinds they turned off.
func (s *Service) items(ctx context.Context, tx pgx.Tx, p authz.Principal, state store.NotificationState, now time.Time) ([]Item, int, error) {
	w := WindowAt(now, s.cfg.TimeZone)
	wants := func(kind string) bool { return !slices.Contains(state.MutedKinds, kind) }
	var in Inputs
	var err error
	if p.Can(api.PermissionPipelineRead) && wants(KindApplicant) {
		jobs, err := s.src.OwnedJobs(ctx, tx, p.UserID)
		if err != nil {
			return nil, 0, err
		}
		if in.Applicants, err = s.src.Applicants(ctx, tx, jobs, w.ApplicantsSince); err != nil {
			return nil, 0, err
		}
	}
	if p.Can(api.PermissionAutomationsTasksRead) && wants(KindTask) {
		if in.Tasks, err = s.src.DueTasks(ctx, tx, p.UserID, w.Today); err != nil {
			return nil, 0, err
		}
	}
	if p.Can(api.PermissionOffersApprove) && wants(KindOfferApproval) {
		if in.Offers, err = s.src.OffersToApprove(ctx, tx, p.UserID); err != nil {
			return nil, 0, err
		}
	}
	if p.Can(api.PermissionInterviewsRead) && wants(KindInterview) {
		if in.Interviews, err = s.src.Interviews(ctx, tx, p.UserID, w.From, w.To); err != nil {
			return nil, 0, err
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
			return nil, 0, err
		}
	}
	items, unseen := Build(in, state.SeenAt, w)
	return items, unseen, nil
}
