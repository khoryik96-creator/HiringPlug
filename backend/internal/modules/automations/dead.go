package automations

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// DeadJobHandlers end a scheduled run whose job ended without running it
// (cancelled after a restore, or stopped for good): scheduled for ever
// otherwise.
func (m *Module) DeadJobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindAutomationRun: m.service.runDied}
}

func (s *Service) runDied(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var j api.AutomationRunJob
	if err := job.Decode(&j); err != nil {
		return nil //nolint:nilerr // an unreadable job names no run
	}
	q := store.New(tx)
	run, err := q.LockRun(ctx, j.RunId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if run.Status != Scheduled || !run.DueAt.Equal(j.DueAt) {
		return nil // ran, or moved to another time (another job)
	}
	why := "Not run: it stopped before it ran."
	if jobqueue.WasCancelledAfterRestore(job) {
		why = "Not run: cancelled after the server was restored (it was waiting in the backup)."
	}
	_, err = q.FinishRun(ctx, store.FinishRunParams{ID: run.ID, Status: Failed, Detail: &why})
	return err
}
