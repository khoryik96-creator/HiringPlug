// Package heartbeat lets each background process (the Go worker, the AI
// worker) say every half minute that it runs and which version
// (server_process, through server_process_seen()), and gives Settings ›
// System what they last said: a worker that stopped, or one still on the old
// version after an update, shows there.
package heartbeat

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Every is how often a process reports; Stale is when one counts as stopped.
const (
	Every = 30 * time.Second
	Stale = 2 * time.Minute
)

// Process names.
const (
	Worker   = "worker"
	AIWorker = "ai-worker"
)

// Beat reports once that the process name runs version since started.
func Beat(ctx context.Context, pool *pgxpool.Pool, name, version string, started time.Time) error {
	_, err := pool.Exec(ctx, `SELECT server_process_seen($1, $2, $3)`, name, version, started)
	return err
}

// Run reports at once and then every Every until ctx ends; a failed report is
// logged and tried again at the next beat.
func Run(ctx context.Context, pool *pgxpool.Pool, name, version string, logger *slog.Logger) {
	started := time.Now()
	tick := time.NewTicker(Every)
	defer tick.Stop()
	for {
		if err := Beat(ctx, pool, name, version, started); err != nil && ctx.Err() == nil {
			logger.Warn("could not report that this process runs", "process", name, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Process is what a kind of process last reported.
type Process struct {
	Name      string
	Version   string
	StartedAt time.Time
	SeenAt    time.Time
}

// Read returns what each process last reported (a pool or a transaction).
func Read(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) (map[string]Process, error) {
	rows, err := db.Query(ctx, `SELECT name, version, started_at, seen_at FROM server_process`)
	if err != nil {
		return nil, err
	}
	out := map[string]Process{}
	for rows.Next() {
		var p Process
		if err := rows.Scan(&p.Name, &p.Version, &p.StartedAt, &p.SeenAt); err != nil {
			rows.Close()
			return nil, err
		}
		out[p.Name] = p
	}
	return out, rows.Err()
}
