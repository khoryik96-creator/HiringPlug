// Package aipaid is the server's paid-AI switch (ai_paid_control, one row
// for the server): paused, no paid AI call is made on the server, through a
// company's own key or the server's provider, for new work (Go: the AI
// modules' access, aiproviders) or work already queued (the AI worker:
// Deps.provider). HP_AI_PAID=off pauses it whatever the row says. Local
// work (text and OCR, PDF copies, search by meaning, reading CVs by the
// rules) goes on.
package aipaid

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB is a pool or a transaction.
type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Switch is the server's paid AI: paused or not, why, when and by whom.
type Switch struct {
	Paused    bool
	Reason    *string
	ChangedAt *time.Time
	ChangedBy *string
}

// Read returns the switch.
func Read(ctx context.Context, db DB) (Switch, error) {
	var s Switch
	err := db.QueryRow(ctx, `SELECT paused, reason, changed_at, changed_by FROM ai_paid_control`).
		Scan(&s.Paused, &s.Reason, &s.ChangedAt, &s.ChangedBy)
	return s, err
}

// Paused reports whether paid AI is paused (the row; HP_AI_PAID is the caller's).
func Paused(ctx context.Context, db DB) (bool, error) {
	var paused bool
	err := db.QueryRow(ctx, `SELECT ai_paid_paused()`).Scan(&paused)
	return paused, err
}

// Set pauses paid AI, or turns it back on, saying why and who.
func Set(ctx context.Context, db DB, paused bool, reason, by string) error {
	_, err := db.Exec(ctx, `UPDATE ai_paid_control SET paused = $1, reason = NULLIF(left($2, 300), ''), changed_at = now(), changed_by = $3`,
		paused, reason, by)
	return err
}
