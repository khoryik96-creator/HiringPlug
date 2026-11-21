package leads

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

type leadView struct {
	lead     store.JobLead
	favorite bool
	evidence *store.JobLeadEvidence
}

func pageLeads[T any](rows []T, limit int, id func(T) string) ([]T, *string) {
	if len(rows) <= limit {
		return rows, nil
	}
	cursor := id(rows[limit-1])
	return rows[:limit], &cursor
}

func (s *Service) workspace(ctx context.Context, p authz.Principal, filter store.ListLeadsParams) ([]store.ListLeadsRow, error) {
	var rows []store.ListLeadsRow
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) (err error) {
		rows, err = store.New(tx).ListLeads(ctx, filter)
		return err
	})
	return rows, err
}

func (s *Service) leadDetail(ctx context.Context, p authz.Principal, id string) (leadView, error) {
	var view leadView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetLead(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("lead")
		}
		if err != nil {
			return err
		}
		view.lead = row
		view.favorite, err = q.LeadFavorite(ctx, id)
		if err != nil {
			return err
		}
		evidence, err := q.GetLeadEvidence(ctx, id)
		if database.IsNoRows(err) {
			return nil
		}
		if err == nil {
			view.evidence = &evidence
		}
		return err
	})
	return view, err
}

func (s *Service) setFavorite(ctx context.Context, p authz.Principal, id string, favorite bool) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		_, err := q.GetLead(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("lead")
		}
		if err != nil {
			return err
		}
		var changed int64
		if favorite {
			changed, err = q.AddLeadFavorite(ctx, id)
		} else {
			changed, err = q.RemoveLeadFavorite(ctx, id)
		}
		if err != nil || changed == 0 {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "job_lead.favorite_changed", EntityType: "job_lead", EntityID: id,
			Changes: map[string]any{"favorite": favorite},
		})
	})
}
