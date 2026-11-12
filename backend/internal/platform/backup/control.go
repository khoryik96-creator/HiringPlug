package backup

// The nightly backup's switch and "Back up now", kept in backup_control (one
// row for the whole server). Settings changes them; the backup service checks
// them every PollEvery, and says it is alive by checking in.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PollEvery is how often the backup service checks the switch and requests.
const PollEvery = 30 * time.Second

// db is what these functions need: a pool or a transaction.
type db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Control is the switch, a pending request and the service's last check-in.
type Control struct {
	Auto          bool
	RequestedAt   *time.Time
	ServiceSeenAt *time.Time
}

// ReadControl returns the switch and request.
func ReadControl(ctx context.Context, q db) (Control, error) {
	var c Control
	err := q.QueryRow(ctx, `SELECT auto, requested_at, service_seen_at FROM backup_control`).
		Scan(&c.Auto, &c.RequestedAt, &c.ServiceSeenAt)
	return c, err
}

// SetAuto turns the nightly backup on or off.
func SetAuto(ctx context.Context, q db, on bool) error {
	_, err := q.Exec(ctx, `UPDATE backup_control SET auto = $1, updated_at = now()`, on)
	return err
}

// Request asks for a backup now (kept if one is already asked for).
func Request(ctx context.Context, q db) error {
	_, err := q.Exec(ctx, `UPDATE backup_control SET requested_at = coalesce(requested_at, now()), updated_at = now()`)
	return err
}

// CheckIn records that the backup service is running, and returns the
// switch and request.
func CheckIn(ctx context.Context, q db) (Control, error) {
	var c Control
	err := q.QueryRow(ctx, `UPDATE backup_control SET service_seen_at = now()
		RETURNING auto, requested_at, service_seen_at`).Scan(&c.Auto, &c.RequestedAt, &c.ServiceSeenAt)
	return c, err
}

// Answered clears a request made before the latest backup started (made or
// failed: a failed one is not tried again every PollEvery; Settings shows why).
func Answered(ctx context.Context, q db) error {
	_, err := q.Exec(ctx, `UPDATE backup_control SET requested_at = NULL
		WHERE requested_at <= (SELECT max(started_at) FROM backup_run)`)
	return err
}

// LastStarted returns when the latest backup started (nil before the first).
func LastStarted(ctx context.Context, q db) (*time.Time, error) {
	var t *time.Time
	err := q.QueryRow(ctx, `SELECT max(started_at) FROM backup_run`).Scan(&t)
	return t, err
}

// Due says whether to make a backup now: one was asked for, or the nightly
// backup is on and the last one started at least every ago (or never). A
// failed backup waits for the next turn too, so a broken one does not repeat
// every PollEvery.
func Due(now time.Time, c Control, lastStarted *time.Time, every time.Duration) bool {
	if c.RequestedAt != nil {
		return true
	}
	return c.Auto && (lastStarted == nil || now.Sub(*lastStarted) >= every)
}
