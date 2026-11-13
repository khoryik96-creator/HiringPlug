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
	// ServiceRunning: it checked in within the last two minutes, by the
	// database's clock (never a browser's).
	ServiceRunning bool
}

// ReadControl returns the switch and request.
func ReadControl(ctx context.Context, q db) (Control, error) {
	var c Control
	err := q.QueryRow(ctx, `SELECT auto, requested_at, service_seen_at,
		coalesce(service_seen_at > now() - interval '2 minutes', false) FROM backup_control`).
		Scan(&c.Auto, &c.RequestedAt, &c.ServiceSeenAt, &c.ServiceRunning)
	return c, err
}

// FailInterrupted marks backups still "running" as failed: the backup service
// calls it when it starts, when none can be running (it was stopped
// mid-backup), so Settings does not wait for them forever.
func FailInterrupted(ctx context.Context, q db) error {
	_, err := q.Exec(ctx, `UPDATE backup_run SET status = 'failed', finished_at = now(),
		error = 'The backup service stopped before this backup was finished.' WHERE status = 'running'`)
	return err
}

// SaveDownload keeps a download token (hashed) for one backup until expires,
// and forgets the expired ones.
func SaveDownload(ctx context.Context, q db, tokenHash, name string, expires time.Time) error {
	if _, err := q.Exec(ctx, `DELETE FROM backup_download WHERE expires_at < now()`); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `INSERT INTO backup_download (token_hash, name, expires_at) VALUES ($1, $2, $3)`, tokenHash, name, expires)
	return err
}

// DownloadAllowed reports whether a token (hashed) is a live one for this backup.
func DownloadAllowed(ctx context.Context, q db, tokenHash, name string) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM backup_download WHERE token_hash = $1 AND name = $2 AND expires_at > now())`,
		tokenHash, name).Scan(&ok)
	return ok, err
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
