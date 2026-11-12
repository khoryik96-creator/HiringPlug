package automations

// The recruiters' tasks: each user sees and ticks off their own.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// TaskView is a task as shown, with what it is about.
type TaskView struct {
	store.RecruiterTask
	App *Application
}

// Tasks returns p's open tasks and those done in the last 7 days.
func (s *Service) Tasks(ctx context.Context, p authz.Principal) ([]TaskView, error) {
	var out []TaskView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListTasks(ctx, p.UserID)
		if err != nil {
			return err
		}
		out, err = s.taskViews(ctx, tx, rows)
		return err
	})
	return out, err
}

// SetTaskDone ticks one of p's tasks off (or back on).
func (s *Service) SetTaskDone(ctx context.Context, p authz.Principal, id string, done bool) (TaskView, error) {
	var out TaskView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).SetTaskDone(ctx, store.SetTaskDoneParams{ID: id, UserID: p.UserID, Done: done})
		if database.IsNoRows(err) {
			return apperr.NotFound("task") // or someone else's
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "task.updated", EntityType: "task", EntityID: id, Changes: map[string]any{"done": done}}); err != nil {
			return err
		}
		views, err := s.taskViews(ctx, tx, []store.RecruiterTask{row})
		if err == nil {
			out = views[0]
		}
		return err
	})
	return out, err
}

func (s *Service) taskViews(ctx context.Context, tx pgx.Tx, rows []store.RecruiterTask) ([]TaskView, error) {
	ids := []string{}
	for _, r := range rows {
		if r.ApplicationID != nil {
			ids = append(ids, *r.ApplicationID)
		}
	}
	apps := map[string]Application{}
	if len(ids) > 0 {
		var err error
		if apps, err = s.deps.Applications.Applications(ctx, tx, ids); err != nil {
			return nil, err
		}
	}
	out := make([]TaskView, len(rows))
	for i, r := range rows {
		out[i] = TaskView{RecruiterTask: r}
		if r.ApplicationID != nil {
			if a, ok := apps[*r.ApplicationID]; ok {
				out[i].App = &a
			}
		}
	}
	return out, nil
}
