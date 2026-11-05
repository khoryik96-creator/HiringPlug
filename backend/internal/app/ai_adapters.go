package app

// The AI allowance shared by the modules that use the AI: one monthly
// allowance per company covers the assistant, CV vs job matching and
// reading CVs.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
)

type tokensSince interface {
	TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error)
}

// aiUse adds up the AI use of several modules. Modules are looked up when
// called (some are built after the ones that need them).
type aiUse struct{ parts func() []tokensSince }

func (u aiUse) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	var total int64
	for _, p := range u.parts() {
		n, err := p.TokensSince(ctx, tx, since)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// cvReadingAI lets the documents module use the AI when it is on and the
// company has allowance left this month.
type cvReadingAI struct {
	all aiUse
	cfg config.AI
	tz  *time.Location
	now func() time.Time
}

func (a cvReadingAI) Allowed(ctx context.Context, tx pgx.Tx) (bool, error) {
	if !a.cfg.Enabled {
		return false, nil
	}
	used, err := a.all.TokensSince(ctx, tx, assistant.MonthStart(a.now(), a.tz))
	return used < a.cfg.MonthlyTokens, err
}
