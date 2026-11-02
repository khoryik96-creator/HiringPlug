package pipeline

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// AddApplied puts a candidate on a job at "applied", in the caller's
// transaction (the careers page's apply form). A candidate already on the job
// stays where they are: added is false.
// channel is where they came from (careers_page, linkedin, ...).
func (m *Module) AddApplied(ctx context.Context, tx pgx.Tx, jobID, candidateID, channel string) (added bool, err error) {
	row, err := store.New(tx).InsertApplication(ctx, store.InsertApplicationParams{
		JobOrderID: jobID, CandidateID: candidateID, Stage: Applied, Channel: &channel,
	})
	if database.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, audit.Record(ctx, tx, audit.Entry{
		Action: "application.created", EntityType: "application", EntityID: row.ID,
		Changes: map[string]any{"jobId": jobID, "candidateId": candidateID, "stage": Applied, "channel": channel},
	})
}
