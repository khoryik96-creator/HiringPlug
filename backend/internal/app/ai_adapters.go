package app

// The AI allowance shared by every module that uses the AI: one monthly limit
// per company (its own, or the server's default) covers them all, checked by
// aiusage, and every AI run is recorded in aiusage's ledger.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiproviders"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant"
)

// aiLedger is aiusage as the AI modules see it: what is left of this month's
// allowance, and where each AI run's tokens are recorded.
type aiLedger struct{ usage *aiusage.Module }

// RecordAI records one AI run's tokens in the caller's transaction.
func (l aiLedger) RecordAI(ctx context.Context, tx pgx.Tx, tenantID, feature, sourceID, runKey string, userID *string, input, output int64) error {
	return l.usage.Record(ctx, tx, tenantID, aiusage.Use{
		Feature: feature, UserID: userID, SourceID: sourceID, RunKey: runKey, Input: input, Output: output,
	})
}

// Remaining is the allowance left for feature, in tokens (0 or less: used
// up): this month's, today's and the feature's own (aiusage.RemainingFor).
func (l aiLedger) Remaining(ctx context.Context, tx pgx.Tx, tenantID, feature string) (int64, error) {
	return l.usage.RemainingFor(ctx, tx, tenantID, feature)
}

// cvReadingAI lets a module use the AI when the company has one and
// allowance left this month, and records what it used.
type cvReadingAI struct {
	aiLedger
	ai *aiproviders.Module
}

func (a cvReadingAI) Allowed(ctx context.Context, tx pgx.Tx, tenantID, feature string) (*api.AiAccess, bool, error) {
	access, on, err := a.ai.Access(ctx, tx)
	if err != nil || !on {
		return nil, false, err
	}
	left, err := a.Remaining(ctx, tx, tenantID, feature)
	return access, left > 0, err
}

// assistantUsage adds this month's use, as the assistant's status shows it.
type assistantUsage struct{ aiLedger }

func (a assistantUsage) ThisMonth(ctx context.Context, tx pgx.Tx, tenantID string) (assistant.MonthUse, error) {
	st, err := a.usage.Status(ctx, tx, tenantID)
	return assistant.MonthUse{UsedTokens: st.UsedTokens, LimitTokens: st.LimitTokens, UsedPercent: st.UsedPercent}, err
}
