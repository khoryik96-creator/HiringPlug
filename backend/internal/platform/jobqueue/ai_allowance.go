package jobqueue

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// EstimatedAITokens is the provisional hold per outstanding AI operation.
// A final request can use the remaining allowance even when it is less than
// the estimate. Actual usage replaces the hold when the result is applied.
const EstimatedAITokens = 1000

// PendingAI locks admissions for this tenant until the caller's transaction
// ends. Read pending work before reading settled usage, so a result that
// lands between the reads can only overestimate usage temporarily.
func PendingAI(ctx context.Context, tx pgx.Tx, tenantID string) (int64, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('ai-allowance:' || $1, 0))", tenantID); err != nil {
		return 0, err
	}
	var pending int64
	err := tx.QueryRow(ctx, "SELECT job_ai_inflight()").Scan(&pending)
	return pending * EstimatedAITokens, err
}
