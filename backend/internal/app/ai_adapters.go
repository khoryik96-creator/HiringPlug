package app

// The AI allowance shared by the modules that use the AI: one monthly
// allowance per company covers the assistant, CV vs job matching, reading
// and formatting CVs, and rewriting blind ads.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiproviders"
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

// cvReadingAI lets the documents module use the AI when the company has
// one and allowance left this month.
type cvReadingAI struct {
	all aiUse
	ai  *aiproviders.Module
	cfg config.AI
	tz  *time.Location
	now func() time.Time
}

func (a cvReadingAI) Allowed(ctx context.Context, tx pgx.Tx) (*api.AiAccess, bool, error) {
	access, on, err := a.ai.Access(ctx, tx)
	if err != nil || !on {
		return nil, false, err
	}
	used, err := a.all.TokensSince(ctx, tx, assistant.MonthStart(a.now(), a.tz))
	return access, used < a.cfg.MonthlyTokens, err
}
